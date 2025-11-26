package gmaps

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosom/scrapemate"
	"github.com/playwright-community/playwright-go"

	"github.com/gosom/google-maps-scraper/exiter"
)

type PlaceJobOptions func(*PlaceJob)

type PlaceJob struct {
	scrapemate.Job

	UsageInResultststs  bool
	ExtractEmail        bool
	ExitMonitor         exiter.Exiter
	ExtractExtraReviews bool

	// Radius filtering
	FilterByRadius bool
	CenterLat      float64
	CenterLon      float64
	RadiusMeters   float64

	// Google Places API enrichment
	GoogleMapsAPIKey string
}

func NewPlaceJob(parentID, langCode, u string, extractEmail, extraExtraReviews bool, opts ...PlaceJobOptions) *PlaceJob {
	const (
		defaultPrio       = scrapemate.PriorityMedium
		defaultMaxRetries = 3
	)

	job := PlaceJob{
		Job: scrapemate.Job{
			ID:         uuid.New().String(),
			ParentID:   parentID,
			Method:     "GET",
			URL:        u,
			URLParams:  map[string]string{"hl": langCode},
			MaxRetries: defaultMaxRetries,
			Priority:   defaultPrio,
		},
	}

	job.UsageInResultststs = true
	job.ExtractEmail = extractEmail
	job.ExtractExtraReviews = extraExtraReviews

	for _, opt := range opts {
		opt(&job)
	}

	return &job
}

func WithPlaceJobExitMonitor(exitMonitor exiter.Exiter) PlaceJobOptions {
	return func(j *PlaceJob) {
		j.ExitMonitor = exitMonitor
	}
}

func WithRadiusFilter(lat, lon, radiusMeters float64) PlaceJobOptions {
	return func(j *PlaceJob) {
		j.FilterByRadius = true
		j.CenterLat = lat
		j.CenterLon = lon
		j.RadiusMeters = radiusMeters
	}
}

func WithGoogleMapsAPIKey(apiKey string) PlaceJobOptions {
	return func(j *PlaceJob) {
		j.GoogleMapsAPIKey = apiKey
	}
}

func (j *PlaceJob) Process(_ context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
		resp.Meta = nil
	}()

	countCompletion := true
	defer func() {
		if countCompletion && j.ExitMonitor != nil {
			j.ExitMonitor.IncrPlacesCompleted(1)
		}
	}()

	// Check if there was an error during browser action (e.g., APP_INITIALIZATION_STATE not found)
	if resp.Error != nil {
		// Don't create email job if place extraction failed
		// Mark as not used in results so it won't be written
		j.UsageInResultststs = false
		return nil, nil, resp.Error
	}

	raw, ok := resp.Meta["json"].([]byte)
	if !ok {
		j.UsageInResultststs = false
		return nil, nil, fmt.Errorf("could not convert to []byte")
	}

	entry, err := EntryFromJSON(raw)
	if err != nil {
		j.UsageInResultststs = false
		return nil, nil, err
	}

	entry.ID = j.ParentID

	if entry.Link == "" {
		entry.Link = j.GetURL()
	}

	// Populate Place ID from the link (extracts from URL)
	entry.populatePlaceIDFromLink()

	// Enrich with Google Places API if API key is provided
	if j.GoogleMapsAPIKey != "" {
		if err := EnrichEntryWithPlaceID(&entry, j.GoogleMapsAPIKey); err != nil {
			// Log error but don't fail - continue with the entry
			fmt.Printf("Warning: Failed to enrich entry with Place ID: %v\n", err)
		}
	}

	// Filter by radius if configured
	if j.FilterByRadius && !entry.isWithinRadius(j.CenterLat, j.CenterLon, j.RadiusMeters) {
		// Place is outside radius, don't include in results
		j.UsageInResultststs = false
		if j.ExitMonitor != nil {
			j.ExitMonitor.IncrPlacesCompleted(1)
		}
		countCompletion = false
		return nil, nil, nil
	}

	allReviewsRaw, ok := resp.Meta["reviews_raw"].(fetchReviewsResponse)
	if ok && len(allReviewsRaw.pages) > 0 {
		entry.AddExtraReviews(allReviewsRaw.pages)
	}

	if j.ExtractEmail && entry.IsWebsiteValidForEmail() {
		opts := []EmailExtractJobOptions{}
		if j.ExitMonitor != nil {
			opts = append(opts, WithEmailJobExitMonitor(j.ExitMonitor))
		}

		emailJob := NewEmailJob(j.ID, &entry, opts...)

		j.UsageInResultststs = false
		countCompletion = false

		return nil, []scrapemate.IJob{emailJob}, nil
	}

	return &entry, nil, err
}

func (j *PlaceJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
	var resp scrapemate.Response

	pageResponse, err := page.Goto(j.GetURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded, // Changed from networkidle - Google Maps never stops loading
		Timeout:   playwright.Float(30000),                   // Reduced timeout since we're not waiting for networkidle
	})
	if err != nil {
		resp.Error = err

		return resp
	}

	clickRejectCookiesIfRequired(page)

	// Wait for the page to be fully interactive
	// Google Maps needs time to populate APP_INITIALIZATION_STATE
	time.Sleep(5 * time.Second)

	resp.URL = pageResponse.URL()
	resp.StatusCode = pageResponse.Status()
	resp.Headers = make(http.Header, len(pageResponse.Headers()))

	for k, v := range pageResponse.Headers() {
		resp.Headers.Add(k, v)
	}

	raw, err := j.extractJSON(page)
	if err != nil {
		resp.Error = err

		return resp
	}

	if resp.Meta == nil {
		resp.Meta = make(map[string]any)
	}

	resp.Meta["json"] = raw

	if j.ExtractExtraReviews {
		reviewCount := j.getReviewCount(raw)
		if reviewCount > 8 { // we have more reviews
			params := fetchReviewsParams{
				page:        page,
				mapURL:      page.URL(),
				reviewCount: reviewCount,
			}

			reviewFetcher := newReviewFetcher(params)

			reviewData, err := reviewFetcher.fetch(ctx)
			if err != nil {
				return resp
			}

			resp.Meta["reviews_raw"] = reviewData
		}
	}

	return resp
}

func (j *PlaceJob) extractJSON(page playwright.Page) ([]byte, error) {
	// Retry mechanism: Google Maps may take time to populate APP_INITIALIZATION_STATE
	// Increased retries and delay to handle slower loading pages
	const maxRetries = 5
	const initialDelay = time.Second * 2
	const maxDelay = time.Second * 5

	var rawI interface{}
	var err error

	for i := 0; i < maxRetries; i++ {
		// Wait before attempting (progressive backoff)
		if i > 0 {
			delay := time.Duration(i) * initialDelay
			if delay > maxDelay {
				delay = maxDelay
			}
			time.Sleep(delay)
		}

		rawI, err = page.Evaluate(js)
		if err != nil {
			// If JavaScript evaluation fails, the page structure may have changed
			// Try to wait and retry instead of failing immediately
			if i < maxRetries-1 {
				continue
			}
			return nil, fmt.Errorf("failed to evaluate JavaScript: %w", err)
		}

		// Check if result is valid
		if rawI != nil {
			if raw, ok := rawI.(string); ok && raw != "" {
				// Success! Got valid string data
				const prefix = `)]}'`
				raw = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
				return []byte(raw), nil
			}
		}
	}

	// All retries exhausted
	if rawI == nil {
		return nil, fmt.Errorf("APP_INITIALIZATION_STATE not found on page after %d retries (Google Maps page structure may have changed or page didn't load properly)", maxRetries)
	}

	return nil, fmt.Errorf("could not convert to string (got type %T) after %d retries - page structure may have changed", rawI, maxRetries)
}

func (j *PlaceJob) getReviewCount(data []byte) int {
	tmpEntry, err := EntryFromJSON(data, true)
	if err != nil {
		return 0
	}

	return tmpEntry.ReviewCount
}

func (j *PlaceJob) UseInResults() bool {
	return j.UsageInResultststs
}

func ctxWait(ctx context.Context, dur time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(dur):
	}
}

const js = `
function parse() {
	// Validate APP_INITIALIZATION_STATE exists and is properly structured
	if (!window.APP_INITIALIZATION_STATE) {
		return null;
	}
	
	if (!Array.isArray(window.APP_INITIALIZATION_STATE) || window.APP_INITIALIZATION_STATE.length < 4) {
		return null;
	}
	
	const appState = window.APP_INITIALIZATION_STATE[3];
	if (!appState || typeof appState !== 'object') {
		return null;
	}
	
	const keys = Object.keys(appState);
	if (keys.length === 0) {
		return null;
	}
	
	const key = keys[0];
	if (appState[key] && Array.isArray(appState[key]) && appState[key].length > 6 && appState[key][6]) {
		return appState[key][6];
	}
	
	return null;
}
`
