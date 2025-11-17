package gmaps

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/scrapemate"
	"github.com/playwright-community/playwright-go"
)

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

	// Radius filtering
	FilterByRadius bool
	RadiusMeters   float64
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

	// Apply options first to get RadiusMeters
	for _, opt := range opts {
		opt(&job)
	}

	// Now construct the search URL with proper radius
	distanceMeters := int(job.RadiusMeters)
	if distanceMeters <= 0 {
		distanceMeters = 2000 // Default 2km
	}

	// Construct the nearby search URL (Google will redirect to proper format)
	job.URL = fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dm",
		url.QueryEscape(category),
		latitude,
		longitude,
		distanceMeters,
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

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(resp.Body)))
	if err != nil {
		return nil, nil, err
	}

	next := make([]scrapemate.IJob, 0)

	jopts := []PlaceJobOptions{}

	if j.ExitMonitor != nil {
		jopts = append(jopts, WithPlaceJobExitMonitor(j.ExitMonitor))
	}

	if j.FilterByRadius {
		jopts = append(jopts, WithRadiusFilter(j.Latitude, j.Longitude, j.RadiusMeters))
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

	if j.ExitMonitor != nil {
		j.ExitMonitor.IncrSeedCompleted(1)
		j.ExitMonitor.IncrPlacesFound(len(next))
	}

	// Log how many places were found
	log.Info(fmt.Sprintf("Nearby search for '%s' found %d places (scrolled %d times, radius: %.0fm)",
		j.Category, len(next), j.MaxDepth, j.RadiusMeters))

	return nil, next, nil
}

func (j *NearbySearchJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
	var resp scrapemate.Response

	// Build the nearby search URL directly
	// Format: https://www.google.com/maps/search/CATEGORY/@LAT,LON,DISTANCE
	// Example: https://www.google.com/maps/search/Restaurants/@24.93584,67.13801,2000m

	distanceMeters := int(j.RadiusMeters)
	if distanceMeters <= 0 {
		distanceMeters = 2000 // Default 2km
	}

	// Construct the simple nearby search URL
	// Google Maps will automatically redirect to the proper format with /data= parameter
	// Simple format: https://www.google.com/maps/search/Restaurants/@24.935840,67.138010,5000m
	// Google redirects to: https://www.google.com/maps/search/Restaurants/@24.93584,67.13801,6318m/data=!3m1!1e3?entry=ttu...
	searchURL := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dm",
		url.QueryEscape(j.Category),
		j.Latitude,
		j.Longitude,
		distanceMeters,
	)

	// Add language parameter
	if j.LangCode != "" {
		searchURL += "?hl=" + j.LangCode
	}

	// Navigate directly to the search nearby URL
	log := scrapemate.GetLoggerFromContext(ctx)
	log.Info(fmt.Sprintf("Navigating to: %s", searchURL))

	pageResponse, err := page.Goto(searchURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})

	if err != nil {
		resp.Error = fmt.Errorf("failed to navigate to search nearby URL: %w", err)
		return resp
	}

	// Handle cookie consent
	if err = clickRejectCookiesIfRequired(page); err != nil {
		resp.Error = fmt.Errorf("failed to handle cookies: %w", err)
		return resp
	}

	// Wait for page to stabilize and dynamic content to load
	// Nearby search loads places dynamically, so wait longer
	time.Sleep(3 * time.Second)

	// Wait for results to load - nearby search needs more time for dynamic content
	time.Sleep(3 * time.Second)

	// NOW check the URL after Google Maps has completed its JavaScript redirect
	finalURL := page.URL()
	if finalURL != searchURL {
		log.Info(fmt.Sprintf("Google Maps redirected to: %s", finalURL))
	} else {
		log.Info(fmt.Sprintf("Final URL: %s", finalURL))
	}

	// Wait for the results feed to appear
	feedSelector := `div[role='feed']`
	_, err = page.WaitForSelector(feedSelector, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(15000),
	})
	if err != nil {
		resp.Error = fmt.Errorf("results feed not found: %w", err)
		return resp
	}

	log.Info(fmt.Sprintf("Starting to scroll (max depth: %d) to find nearby places...", j.MaxDepth))

	// Scroll through the results
	scrollAttempts, err := scroll(ctx, page, j.MaxDepth, feedSelector)
	if err != nil {
		resp.Error = fmt.Errorf("failed to scroll results: %w", err)
		return resp
	}

	log.Info(fmt.Sprintf("Completed scrolling after %d attempts", scrollAttempts))

	// Wait for place cards to appear in the feed after scrolling
	// This ensures JavaScript has finished rendering the results
	placeCardSelector := `div[role='feed'] a[href*='/maps/place/']`
	_, err = page.WaitForSelector(placeCardSelector, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000),
		State:   playwright.WaitForSelectorStateVisible,
	})
	if err != nil {
		log.Info(fmt.Sprintf("Warning: Place cards not immediately visible after scrolling: %v", err))
		// Continue anyway - there might be results or we'll discover the issue in Process
	} else {
		log.Info("Place cards detected in feed")
	}

	// Additional wait for JavaScript rendering to complete
	time.Sleep(2 * time.Second)

	// Get the page content
	body, err := page.Content()
	if err != nil {
		resp.Error = fmt.Errorf("failed to get page content: %w", err)
		return resp
	}

	resp.URL = page.URL()
	resp.StatusCode = pageResponse.Status()
	resp.Headers = make(http.Header, len(pageResponse.Headers()))

	for k, v := range pageResponse.Headers() {
		resp.Headers.Add(k, v)
	}

	resp.Body = []byte(body)

	return resp
}

// Ensure NearbySearchJob implements the necessary interfaces
var _ scrapemate.IJob = (*NearbySearchJob)(nil)
