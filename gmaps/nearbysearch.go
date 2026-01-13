package gmaps

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/scrapemate"
)

// nearbyScrollCallback is called after each scroll iteration
type nearbyScrollCallback func(page scrapemate.BrowserPage, depth int) (int, error)

type NearbySearchJobOptions func(*NearbySearchJob)

type NearbySearchJob struct {
	scrapemate.Job

	Latitude     float64
	Longitude    float64
	Category     string
	MaxDepth     int
	LangCode     string
	ExtractEmail bool

	Deduper             deduper.Deduper
	ExitMonitor         exiter.Exiter
	ExtractExtraReviews bool

	// Radius filtering - for filtering results by distance
	FilterByRadius bool
	RadiusMeters   float64

	// Zoom for URL - for setting the map view distance (separate from filtering)
	ZoomMeters float64

	// Google Places API enrichment
	GoogleMapsAPIKey string

	// Progressive extraction - stores URLs found during scrolling
	extractedURLs []string
	urlsMutex     sync.Mutex
}

func NewNearbySearchJob(
	id, langCode string,
	latitude, longitude float64,
	category string,
	maxDepth int,
	extractEmail bool,
	opts ...NearbySearchJobOptions,
) *NearbySearchJob {
	const (
		maxRetries = 3
		prio       = scrapemate.PriorityLow
	)

	if id == "" {
		id = uuid.New().String()
	}

	job := NearbySearchJob{
		Job: scrapemate.Job{
			ID:         id,
			Method:     http.MethodGet,
			URL:        "", // Will be set after applying options
			URLParams:  map[string]string{"hl": langCode},
			MaxRetries: maxRetries,
			Priority:   prio,
		},
		Latitude:     latitude,
		Longitude:    longitude,
		Category:     category,
		MaxDepth:     maxDepth,
		LangCode:     langCode,
		ExtractEmail: extractEmail,
	}

	// Apply options first to get RadiusMeters and ZoomMeters
	for _, opt := range opts {
		opt(&job)
	}

	// Use ZoomMeters for URL generation (map view), default to 2000m if not set
	zoomMeters := int(job.ZoomMeters)
	if zoomMeters <= 0 {
		zoomMeters = 2000 // Default 2km for map view
	}

	// Construct the nearby search URL (Google will redirect to proper format)
	job.URL = fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dm",
		url.QueryEscape(category),
		latitude,
		longitude,
		zoomMeters,
	)

	return &job
}

func WithNearbyDeduper(d deduper.Deduper) NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.Deduper = d
	}
}

func WithNearbyExitMonitor(e exiter.Exiter) NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.ExitMonitor = e
	}
}

func WithNearbyExtraReviews() NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.ExtractExtraReviews = true
	}
}

func WithNearbyRadiusFiltering(lat, lon, radiusMeters float64) NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.FilterByRadius = true
		j.RadiusMeters = radiusMeters
		// Also update the center coordinates for filtering
		j.Latitude = lat
		j.Longitude = lon
	}
}

func WithNearbyZoom(zoomMeters float64) NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.ZoomMeters = zoomMeters
	}
}

func WithNearbyGoogleMapsAPIKey(apiKey string) NearbySearchJobOptions {
	return func(j *NearbySearchJob) {
		j.GoogleMapsAPIKey = apiKey
	}
}

func (j *NearbySearchJob) UseInResults() bool {
	return false
}

func (j *NearbySearchJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
		resp.Meta = nil
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	next := make([]scrapemate.IJob, 0)

	jopts := []PlaceJobOptions{}

	if j.ExitMonitor != nil {
		jopts = append(jopts, WithPlaceJobExitMonitor(j.ExitMonitor))
	}

	if j.FilterByRadius {
		jopts = append(jopts, WithRadiusFilter(j.Latitude, j.Longitude, j.RadiusMeters))
	}

	if j.GoogleMapsAPIKey != "" {
		jopts = append(jopts, WithGoogleMapsAPIKey(j.GoogleMapsAPIKey))
	}

	// Check if we used progressive extraction
	if string(resp.Body) == "progressive_extraction_completed" {
		// Use progressively extracted URLs from BrowserActions
		j.urlsMutex.Lock()
		urls := make([]string, len(j.extractedURLs))
		copy(urls, j.extractedURLs)
		j.urlsMutex.Unlock()

		for _, href := range urls {
			nextJob := NewPlaceJob(j.ID, j.LangCode, href, j.ExtractEmail, j.ExtractExtraReviews, jopts...)
			if j.Deduper == nil || j.Deduper.AddIfNotExists(ctx, href) {
				next = append(next, nextJob)
			}
		}
	} else {
		// Fallback: parse HTML document (for backward compatibility)
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
		if err != nil {
			return nil, nil, err
		}

		// Extract place links from the feed
		doc.Find(`div[role=feed] div[jsaction]>a`).Each(func(_ int, s *goquery.Selection) {
			if href := s.AttrOr("href", ""); href != "" {
				nextJob := NewPlaceJob(j.ID, j.LangCode, href, j.ExtractEmail, j.ExtractExtraReviews, jopts...)
				if j.Deduper == nil || j.Deduper.AddIfNotExists(ctx, href) {
					next = append(next, nextJob)
				}
			}
		})

		// Debug: If no places found with primary selector, try alternative selectors
		if len(next) == 0 {
			log.Info("No places found with primary selector, checking alternatives...")
			
			// Try direct href selector
			altCount := 0
			doc.Find(`a[href*='/maps/place/']`).Each(func(_ int, s *goquery.Selection) {
				altCount++
			})
			log.Info(fmt.Sprintf("Alternative selector found %d place links", altCount))
			
			// Try the alternative and use it if it finds results
			if altCount > 0 {
				doc.Find(`a[href*='/maps/place/']`).Each(func(_ int, s *goquery.Selection) {
					if href := s.AttrOr("href", ""); href != "" {
						nextJob := NewPlaceJob(j.ID, j.LangCode, href, j.ExtractEmail, j.ExtractExtraReviews, jopts...)
						if j.Deduper == nil || j.Deduper.AddIfNotExists(ctx, href) {
							next = append(next, nextJob)
						}
					}
				})
			}
		}
	}

	if j.ExitMonitor != nil {
		j.ExitMonitor.IncrSeedCompleted(1)
		j.ExitMonitor.IncrPlacesFound(len(next))
	}

	// Log how many places were found
	log.Info(fmt.Sprintf("Nearby search for '%s' found %d places (scrolled %d times, radius: %.0fm)",
		j.Category, len(next), j.MaxDepth, j.RadiusMeters))

	return nil, next, nil
}

func (j *NearbySearchJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
	var resp scrapemate.Response

	// Build the TRUE nearby search URL (proximity-based, not relevance-based)
	// Format: https://www.google.com/maps/search/CATEGORY/@LAT,LON,ZOOMm/data=!3m2!1e3!4b1!4m7!2m6!3m5!...
	// The key is the data parameter with !1e3 which enables proximity sorting

	// Use ZoomMeters for map view (URL generation)
	zoomMeters := int(j.ZoomMeters)
	if zoomMeters <= 0 {
		zoomMeters = 2000 // Default 2km for map view
	}

	// Construct the TRUE nearby search URL with proximity sorting (!1e3)
	// This mimics right-click "Search Nearby" behavior which shows CLOSEST places first
	// Format breakdown:
	// - @LAT,LON,ZOOMm = center point and zoom distance (affects map view)
	// - !3m2!1e3!4b1 = enables nearby/proximity mode (1e3 is key)
	// - !4m7!2m6!3m5!1sCATEGORY!2sLAT,LON!4m2!1dLON!2dLAT = search parameters
	encodedCategory := url.QueryEscape(j.Category)

	// Build the data parameter for true nearby search
	// !1e3 is crucial - it enables proximity-based sorting instead of relevance
	dataParam := fmt.Sprintf("!3m1!1e3!4m7!2m6!3m5!1s%s!2s%.7f,+%.7f!4m2!1d%.7f!2d%.7f",
		encodedCategory,
		j.Latitude,
		j.Longitude,
		j.Longitude,
		j.Latitude,
	)

	searchURL := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dm/data=%s",
		encodedCategory,
		j.Latitude,
		j.Longitude,
		zoomMeters,
		dataParam,
	)

	// Add language parameter
	if j.LangCode != "" {
		searchURL += "?hl=" + j.LangCode
	}

	// Navigate directly to the search nearby URL
	log := scrapemate.GetLoggerFromContext(ctx)
	log.Info(fmt.Sprintf("Navigating to initial URL: %s", searchURL))
	log.Info(fmt.Sprintf("Using zoom: %dm for map view, radius: %.0fm for filtering results", zoomMeters, j.RadiusMeters))

	pageResponse, err := page.Goto(searchURL, scrapemate.WaitUntilDOMContentLoaded)
	if err != nil {
		resp.Error = fmt.Errorf("failed to navigate to search nearby URL: %w", err)
		return resp
	}

	// Handle cookie consent
	clickRejectCookiesIfRequired(page)

	// Wait for page to stabilize and dynamic content to load
	// Nearby search loads places dynamically, so wait longer
	time.Sleep(3 * time.Second)

	// Wait for results to load - nearby search needs more time for dynamic content
	time.Sleep(3 * time.Second)

	// NOW check the URL after Google Maps has completed its JavaScript redirect
	finalURL := page.URL()
	if finalURL != searchURL {
		log.Info(fmt.Sprintf("✓ Google Maps redirected to final URL: %s", finalURL))
	} else {
		log.Info(fmt.Sprintf("✓ Final URL (no redirect): %s", finalURL))
	}

	// Wait for the results feed to appear
	feedSelector := `div[role='feed']`
	err = page.WaitForSelector(feedSelector, 15*time.Second)
	if err != nil {
		resp.Error = fmt.Errorf("results feed not found: %w", err)
		return resp
	}

	log.Info(fmt.Sprintf("Starting to scroll (max depth: %d) to find nearby places...", j.MaxDepth))

	// Create callback to extract places progressively during scrolling
	extractCallback := func(page scrapemate.BrowserPage, depth int) (int, error) {
		body, err := page.Content()
		if err != nil {
			return 0, err
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
		if err != nil {
			return 0, err
		}

		// Track URLs we've seen before this iteration
		seenBefore := make(map[string]bool)
		j.urlsMutex.Lock()
		for _, u := range j.extractedURLs {
			seenBefore[u] = true
		}
		j.urlsMutex.Unlock()

		newURLs := []string{}

		// Extract place URLs from current page state
		doc.Find(`div[role=feed] div[jsaction]>a`).Each(func(_ int, s *goquery.Selection) {
			if href := s.AttrOr("href", ""); href != "" {
				if !seenBefore[href] {
					newURLs = append(newURLs, href)
				}
			}
		})

		// Try alternative selector if primary found nothing
		if len(newURLs) == 0 {
			doc.Find(`a[href*='/maps/place/']`).Each(func(_ int, s *goquery.Selection) {
				if href := s.AttrOr("href", ""); href != "" {
					if !seenBefore[href] {
						newURLs = append(newURLs, href)
					}
				}
			})
		}

		if len(newURLs) > 0 {
			j.urlsMutex.Lock()
			j.extractedURLs = append(j.extractedURLs, newURLs...)
			j.urlsMutex.Unlock()

			log.Info(fmt.Sprintf("Depth %d: found %d new places (%d total)", depth, len(newURLs), len(j.extractedURLs)))
		}

		return len(newURLs), nil
	}

	// Scroll through the results with progressive extraction
	scrollAttempts, err := scrollWithCallback(ctx, page, j.MaxDepth, feedSelector, extractCallback)
	if err != nil {
		resp.Error = fmt.Errorf("failed to scroll results: %w", err)
		return resp
	}

	log.Info(fmt.Sprintf("Completed scrolling after %d attempts, found %d total places", scrollAttempts, len(j.extractedURLs)))

	// Store the extracted URLs marker instead of full HTML
	resp.Body = []byte("progressive_extraction_completed")

	resp.URL = pageResponse.URL
	resp.StatusCode = pageResponse.StatusCode
	resp.Headers = pageResponse.Headers

	return resp
}

// scrollWithCallback scrolls through results and calls callback after each scroll
func scrollWithCallback(ctx context.Context,
	page scrapemate.BrowserPage,
	maxDepth int,
	scrollSelector string,
	callback nearbyScrollCallback,
) (int, error) {
	expr := `async () => {
		const el = document.querySelector("` + scrollSelector + `");
		if (!el) return -1;

		el.scrollTop = el.scrollHeight;

		return new Promise((resolve, reject) => {
			setTimeout(() => {
				resolve(el.scrollHeight);
			}, %d);
		});
	}`

	var currentScrollHeight int
	waitTime := 100.
	cnt := 0

	const (
		timeout  = 500
		maxWait2 = 2000
	)

	for i := 0; i < maxDepth; i++ {
		cnt++

		select {
		case <-ctx.Done():
			return cnt, nil
		default:
		}

		raw, err := page.Eval(fmt.Sprintf(expr, timeout))
		if err != nil {
			return cnt, err
		}

		var newScrollHeight int
		switch v := raw.(type) {
		case float64:
			newScrollHeight = int(v)
		case int:
			newScrollHeight = v
		default:
			newScrollHeight = -1
		}

		if newScrollHeight == -1 {
			return cnt, nil
		}

		// Call the callback to extract data at this depth
		if callback != nil {
			_, _ = callback(page, i)
		}

		if newScrollHeight == currentScrollHeight {
			if waitTime > maxWait2 {
				return cnt, nil
			}
			waitTime *= 1.5
		} else {
			waitTime = 100.
		}

		currentScrollHeight = newScrollHeight
	}

	return cnt, nil
}

// Ensure NearbySearchJob implements the necessary interfaces
var _ scrapemate.IJob = (*NearbySearchJob)(nil)
