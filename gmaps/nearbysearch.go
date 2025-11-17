package gmaps

import (
	"context"
	"fmt"
	"net/http"
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

	// Start at Google Maps centered on the coordinates with appropriate zoom
	mapURL := fmt.Sprintf("https://www.google.com/maps/@%f,%f,15z", latitude, longitude)

	job := NearbySearchJob{
		Job: scrapemate.Job{
			ID:         id,
			Method:     http.MethodGet,
			URL:        mapURL,
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

	for _, opt := range opts {
		opt(&job)
	}

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

	if j.ExitMonitor != nil {
		j.ExitMonitor.IncrSeedCompleted(1)
		j.ExitMonitor.IncrPlacesFound(len(next))
	}

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

	// Construct the search nearby URL
	searchURL := fmt.Sprintf("https://www.google.com/maps/search/%s/@%f,%f,%dm",
		j.Category,
		j.Latitude,
		j.Longitude,
		distanceMeters,
	)

	// Add language parameter
	if j.LangCode != "" {
		searchURL += "?hl=" + j.LangCode
	}

	// Navigate directly to the search nearby URL
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

	// Wait for page to stabilize
	time.Sleep(2 * time.Second)

	// Wait for results to load
	time.Sleep(2 * time.Second)

	// Wait for the results feed to appear
	feedSelector := `div[role='feed']`
	_, err = page.WaitForSelector(feedSelector, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000),
	})
	if err != nil {
		resp.Error = fmt.Errorf("results feed not found: %w", err)
		return resp
	}

	// Scroll through the results
	_, err = scroll(ctx, page, j.MaxDepth, feedSelector)
	if err != nil {
		resp.Error = fmt.Errorf("failed to scroll results: %w", err)
		return resp
	}

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
