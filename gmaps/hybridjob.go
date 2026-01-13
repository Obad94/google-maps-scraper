package gmaps

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/scrapemate"
)

type HybridJobOptions func(*HybridJob)

// HybridJob combines initial search and nearby search
// Phase 1: Browser search to get initial results with coordinates
// Phase 2: For each result's location, create NearbySearchJob to expand coverage
type HybridJob struct {
	scrapemate.Job

	// Search parameters
	Query    string
	LangCode string
	Lat      float64
	Lon      float64
	ZoomLvl  int     // Original zoom level (1-21)
	Radius   float64 // Radius in meters
	MaxDepth int     // For nearby search scrolling

	// Options
	ExtractEmail        bool
	ExtractExtraReviews bool
	GoogleMapsAPIKey    string

	// Internal
	Deduper     deduper.Deduper
	ExitMonitor exiter.Exiter

	// Computed zoom in meters for nearby search
	ZoomMeters int

	// Store extracted seed locations from BrowserActions
	seedLocations []seedLocation
	seedMutex     sync.Mutex
}

// seedLocation stores coordinates extracted from search results
type seedLocation struct {
	URL       string
	Title     string
	Latitude  float64
	Longitude float64
}

func NewHybridJob(
	id string,
	langCode string,
	query string,
	lat, lon float64,
	zoomLvl int,
	radius float64,
	maxDepth int,
	extractEmail bool,
	opts ...HybridJobOptions,
) *HybridJob {
	const (
		maxRetries = 3
		prio       = scrapemate.PriorityHigh
	)

	if id == "" {
		id = uuid.New().String()
	}

	// Convert zoom level to meters for nearby search phase
	zoomMeters := convertZoomToMeters(zoomLvl, lat)

	// Build browser-compatible Google Maps search URL
	searchURL := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dz",
		url.QueryEscape(query),
		lat,
		lon,
		zoomLvl,
	)

	job := &HybridJob{
		Job: scrapemate.Job{
			ID:         id,
			Method:     http.MethodGet,
			URL:        searchURL,
			URLParams:  map[string]string{"hl": langCode},
			MaxRetries: maxRetries,
			Priority:   prio,
		},
		Query:        query,
		LangCode:     langCode,
		Lat:          lat,
		Lon:          lon,
		ZoomLvl:      zoomLvl,
		Radius:       radius,
		MaxDepth:     maxDepth,
		ExtractEmail: extractEmail,
		ZoomMeters:   zoomMeters,
	}

	// Apply options
	for _, opt := range opts {
		opt(job)
	}

	return job
}

// convertZoomToMeters converts Google Maps zoom level (1-21) to approximate meters
// Based on Web Mercator projection: meters_per_pixel = 156543.03392 * cos(latitude) / 2^zoom
// Uses 800px viewport width for search area calculation
func convertZoomToMeters(zoomLevel int, latitude float64) int {
	if zoomLevel < 1 || zoomLevel > 21 {
		return 2000 // Default 2km
	}

	// Standard Web Mercator formula for meters per pixel
	metersPerPixel := 156543.03392 * math.Cos(latitude*math.Pi/180.0) / float64(uint(1)<<uint(zoomLevel))

	// Use 800px as effective viewport width for search area
	const viewportWidth = 800.0
	meters := int(metersPerPixel * viewportWidth)

	// Clamp to sensible values for nearby search
	// Minimum: 51m (required by nearby mode)
	// Maximum: 2000m (larger values cause Google Maps to not load feed properly)
	if meters < 51 {
		meters = 51
	}
	if meters > 2000 {
		meters = 2000
	}

	return meters
}

func WithHybridDeduper(d deduper.Deduper) HybridJobOptions {
	return func(j *HybridJob) {
		j.Deduper = d
	}
}

func WithHybridExitMonitor(e exiter.Exiter) HybridJobOptions {
	return func(j *HybridJob) {
		j.ExitMonitor = e
	}
}

func WithHybridExtraReviews() HybridJobOptions {
	return func(j *HybridJob) {
		j.ExtractExtraReviews = true
	}
}

func WithHybridGoogleMapsAPIKey(apiKey string) HybridJobOptions {
	return func(j *HybridJob) {
		j.GoogleMapsAPIKey = apiKey
	}
}

func (j *HybridJob) UseInResults() bool {
	return false
}

func (j *HybridJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
		resp.Meta = nil
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	// Log hybrid job parameters
	log.Info(fmt.Sprintf("[HYBRID] Processing results for query '%s'", j.Query))
	log.Info(fmt.Sprintf("[HYBRID] Parameters: center=(%.6f, %.6f), zoom=%dz, radius=%.0fm, depth=%d",
		j.Lat, j.Lon, j.ZoomLvl, j.Radius, j.MaxDepth))
	log.Info(fmt.Sprintf("[HYBRID] Converted zoom %dz to %dm for nearby search phase", j.ZoomLvl, j.ZoomMeters))

	// Get seed locations extracted by BrowserActions
	j.seedMutex.Lock()
	seeds := make([]seedLocation, len(j.seedLocations))
	copy(seeds, j.seedLocations)
	j.seedMutex.Unlock()

	log.Info(fmt.Sprintf("[HYBRID] Phase 1 complete: Found %d seed locations from browser search", len(seeds)))

	// Handle 0 results case
	if len(seeds) == 0 {
		log.Info("[HYBRID] No seed locations found")
		log.Info("[HYBRID] Possible reasons: sparse area, restrictive radius, or no matches for query")
		log.Info("[HYBRID] Skipping phase 2 (nearby search) - no seed locations available")

		if j.ExitMonitor != nil {
			j.ExitMonitor.IncrSeedCompleted(1)
		}

		return nil, nil, nil
	}

	// Phase 2: Create NearbySearchJob for each seed location
	log.Info("[HYBRID] Phase 2: Creating nearby search jobs for each seed location...")

	next := make([]scrapemate.IJob, 0, len(seeds)*2)
	nearbyJobCount := 0
	skippedCount := 0

	jopts := []PlaceJobOptions{}
	if j.ExitMonitor != nil {
		jopts = append(jopts, WithPlaceJobExitMonitor(j.ExitMonitor))
	}
	if j.Radius > 0 {
		jopts = append(jopts, WithRadiusFilter(j.Lat, j.Lon, j.Radius))
	}
	if j.GoogleMapsAPIKey != "" {
		jopts = append(jopts, WithGoogleMapsAPIKey(j.GoogleMapsAPIKey))
	}

	for i, seed := range seeds {
		// Skip entries without valid coordinates
		if seed.Latitude == 0 && seed.Longitude == 0 {
			log.Info(fmt.Sprintf("[HYBRID] Skipping entry %d: missing coordinates", i+1))
			skippedCount++
			continue
		}

		opts := []NearbySearchJobOptions{}

		if j.Deduper != nil {
			opts = append(opts, WithNearbyDeduper(j.Deduper))
		}

		if j.ExitMonitor != nil {
			opts = append(opts, WithNearbyExitMonitor(j.ExitMonitor))
		}

		if j.ExtractExtraReviews {
			opts = append(opts, WithNearbyExtraReviews())
		}

		if j.Radius > 0 {
			opts = append(opts, WithNearbyRadiusFiltering(seed.Latitude, seed.Longitude, j.Radius))
		}

		if j.ZoomMeters > 0 {
			opts = append(opts, WithNearbyZoom(float64(j.ZoomMeters)))
		}

		if j.GoogleMapsAPIKey != "" {
			opts = append(opts, WithNearbyGoogleMapsAPIKey(j.GoogleMapsAPIKey))
		}

		// Create nearby job with the same query as the original search
		nearbyJob := NewNearbySearchJob(
			fmt.Sprintf("%s-nearby-%d", j.ID, i),
			j.LangCode,
			seed.Latitude,
			seed.Longitude,
			j.Query,
			j.MaxDepth,
			j.ExtractEmail,
			opts...,
		)

		next = append(next, nearbyJob)
		nearbyJobCount++

		log.Info(fmt.Sprintf("[HYBRID] Created nearby job %d/%d: '%s' at (%.6f, %.6f) with %dm zoom, %d depth",
			nearbyJobCount, len(seeds)-skippedCount, seed.Title, seed.Latitude, seed.Longitude, j.ZoomMeters, j.MaxDepth))

		// Also create PlaceJob for this seed location
		if seed.URL != "" {
			placeJob := NewPlaceJob(j.ID, j.LangCode, seed.URL, j.ExtractEmail, j.ExtractExtraReviews, jopts...)
			if j.Deduper == nil || j.Deduper.AddIfNotExists(ctx, seed.URL) {
				next = append(next, placeJob)
			}
		}
	}

	if j.ExitMonitor != nil {
		j.ExitMonitor.IncrSeedCompleted(1)
		j.ExitMonitor.IncrPlacesFound(len(seeds))
	}

	log.Info(fmt.Sprintf("[HYBRID] Phase 2 complete: Created %d nearby search jobs", nearbyJobCount))
	log.Info(fmt.Sprintf("[HYBRID] Total jobs queued: %d (each nearby job will scroll %d times for more results)",
		len(next), j.MaxDepth))
	if skippedCount > 0 {
		log.Info(fmt.Sprintf("[HYBRID] Skipped %d entries due to missing coordinates", skippedCount))
	}

	return nil, next, nil
}

// BrowserActions navigates to search page, extracts coordinates from visible results
func (j *HybridJob) BrowserActions(ctx context.Context, page scrapemate.BrowserPage) scrapemate.Response {
	var resp scrapemate.Response

	log := scrapemate.GetLoggerFromContext(ctx)
	log.Info(fmt.Sprintf("[HYBRID] Phase 1: Navigating to search for '%s'", j.Query))

	// Navigate to the search URL
	pageResponse, err := page.Goto(j.GetFullURL(), scrapemate.WaitUntilDOMContentLoaded)
	if err != nil {
		resp.Error = fmt.Errorf("failed to navigate: %w", err)
		return resp
	}

	// Handle cookie consent
	clickRejectCookiesIfRequired(page)

	// Wait for page to stabilize
	time.Sleep(2 * time.Second)

	// Wait for the results feed to appear
	feedSelector := `div[role='feed']`
	err = page.WaitForSelector(feedSelector, 10*time.Second)
	if err != nil {
		// Check if redirected to single place
		if strings.Contains(page.URL(), "/maps/place/") {
			log.Info("[HYBRID] Single result found, extracting coordinates from URL")
			coords := extractCoordsFromURL(page.URL())
			if coords.Latitude != 0 || coords.Longitude != 0 {
				j.seedMutex.Lock()
				j.seedLocations = append(j.seedLocations, seedLocation{
					URL:       page.URL(),
					Latitude:  coords.Latitude,
					Longitude: coords.Longitude,
				})
				j.seedMutex.Unlock()
			}
			resp.StatusCode = pageResponse.StatusCode
			resp.URL = page.URL()
			return resp
		}
		resp.Error = fmt.Errorf("results feed not found: %w", err)
		return resp
	}

	// Extract place links and coordinates from visible results (no deep scrolling)
	// Just get the first batch of results as seed locations
	body, err := page.Content()
	if err != nil {
		resp.Error = fmt.Errorf("failed to get page content: %w", err)
		return resp
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		resp.Error = fmt.Errorf("failed to parse HTML: %w", err)
		return resp
	}

	// Extract place URLs and their coordinates
	seenURLs := make(map[string]bool)
	doc.Find(`div[role=feed] div[jsaction]>a`).Each(func(_ int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		if href == "" || seenURLs[href] {
			return
		}
		seenURLs[href] = true

		// Extract coordinates from the URL
		coords := extractCoordsFromURL(href)

		// Get title from aria-label
		title := s.AttrOr("aria-label", "")

		j.seedMutex.Lock()
		j.seedLocations = append(j.seedLocations, seedLocation{
			URL:       href,
			Title:     title,
			Latitude:  coords.Latitude,
			Longitude: coords.Longitude,
		})
		j.seedMutex.Unlock()
	})

	log.Info(fmt.Sprintf("[HYBRID] Extracted %d seed locations from search results", len(j.seedLocations)))

	resp.StatusCode = pageResponse.StatusCode
	resp.URL = pageResponse.URL
	resp.Headers = pageResponse.Headers

	return resp
}

// extractCoordsFromURL extracts latitude and longitude from a Google Maps place URL
// URLs look like: https://www.google.com/maps/place/.../@LAT,LON,17z/...
func extractCoordsFromURL(urlStr string) struct{ Latitude, Longitude float64 } {
	result := struct{ Latitude, Longitude float64 }{}

	// Pattern to match @LAT,LON in URL
	re := regexp.MustCompile(`@(-?\d+\.?\d*),(-?\d+\.?\d*)`)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) >= 3 {
		fmt.Sscanf(matches[1], "%f", &result.Latitude)
		fmt.Sscanf(matches[2], "%f", &result.Longitude)
	}

	return result
}

// Ensure HybridJob implements the necessary interfaces
var _ scrapemate.IJob = (*HybridJob)(nil)
