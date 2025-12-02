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
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(60000), // 60 seconds for slow proxy connections
	})
	if err != nil {
		resp.Error = err

		return resp
	}

	// Wait 1 second for any final redirects to complete (Google Maps SPAs)
	page.WaitForTimeout(1000)

	clickRejectCookiesIfRequired(page)

	// Check if we were redirected to a consent page after initial navigation
	currentURL := page.URL()
	if strings.Contains(currentURL, "consent.google.com") {
		fmt.Printf("DEBUG: Redirected to consent page, attempting to handle and retry...\n")
		handleConsentPage(page)

		// After handling consent, retry navigation to original URL
		pageResponse, err = page.Goto(j.GetURL(), playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateDomcontentloaded,
			Timeout:   playwright.Float(15000),
		})
		if err != nil {
			resp.Error = fmt.Errorf("failed to navigate after consent: %w", err)
			return resp
		}

		// Wait for final redirect after consent
		page.WaitForTimeout(1000)

		// Check for consent again after retry (in case of persistent redirects)
		clickRejectCookiesIfRequired(page)
	}

	const defaultTimeout = 5000

	err = page.WaitForURL(page.URL(), playwright.PageWaitForURLOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(defaultTimeout),
	})
	if err != nil {
		resp.Error = err

		return resp
	}

	// For place_id URLs, wait for the redirect to complete to the actual place URL
	// This is critical during browser cold-start when multiple pages load simultaneously
	// The final URL should contain data markers like !1s (hex ID) and !16s (category)
	if strings.Contains(j.GetURL(), "place_id:") {
		// Wait for the URL to change from place_id: format to actual place URL
		const maxWaitRedirect = 20 * time.Second
		const checkInterval = 300 * time.Millisecond
		startTime := time.Now()
		redirectComplete := false
		
		for time.Since(startTime) < maxWaitRedirect {
			currentURL := page.URL()
			// Check if we've been redirected to an actual place URL with full data
			// The canonical format includes !1s0x... (hex ID) and !16s (category marker)
			if !strings.Contains(currentURL, "place_id:") && 
			   strings.Contains(currentURL, "/maps/place/") &&
			   (strings.Contains(currentURL, "!1s0x") || strings.Contains(currentURL, "!16s")) {
				redirectComplete = true
				break
			}
			time.Sleep(checkInterval)
		}
		
		// If redirect completed, wait for networkidle and data to populate
		if redirectComplete {
			page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State:   playwright.LoadStateNetworkidle,
				Timeout: playwright.Float(10000),
			})
			time.Sleep(500 * time.Millisecond)
		} else {
			// Redirect didn't complete properly - log for debugging
			fmt.Printf("DEBUG: Redirect timeout for %s, current URL: %s\n", j.GetURL(), page.URL())
		}
	}

	// Wait for network to settle - helps with data population on redirected pages
	page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000),
	})

	resp.URL = pageResponse.URL()
	resp.StatusCode = pageResponse.Status()
	resp.Headers = make(http.Header, len(pageResponse.Headers()))

	for k, v := range pageResponse.Headers() {
		resp.Headers.Add(k, v)
	}

	raw, err := j.extractJSON(page)
	if err != nil {
		// Diagnostic: capture page state when extraction fails
		title, _ := page.Title()
		url := page.URL()
		fmt.Printf("DEBUG: Extraction failed for %s\n", j.GetURL())
		fmt.Printf("DEBUG: Page title: %s\n", title)
		fmt.Printf("DEBUG: Current URL: %s\n", url)

		// Check what JavaScript state exists
		jsCheck, _ := page.Evaluate(`
			(function() {
				let result = {
					hasAppState: typeof window.APP_INITIALIZATION_STATE !== 'undefined',
					isArray: Array.isArray(window.APP_INITIALIZATION_STATE),
					length: window.APP_INITIALIZATION_STATE ? window.APP_INITIALIZATION_STATE.length : 0,
					hasWindow: typeof window !== 'undefined',
					hasDocument: typeof document !== 'undefined',
					readyState: document.readyState,
					foundJsonPrefix: false,
					allStringInfo: []
				};
				
				// Debug: comprehensively look for all strings in APP_INITIALIZATION_STATE
				if (window.APP_INITIALIZATION_STATE && Array.isArray(window.APP_INITIALIZATION_STATE)) {
					for (let i = 0; i < window.APP_INITIALIZATION_STATE.length; i++) {
						const appState = window.APP_INITIALIZATION_STATE[i];
						if (!appState || typeof appState !== 'object') continue;
						
						const keys = Object.keys(appState);
						if (keys.length === 0) continue;
						
						const key = keys[0];
						if (!appState[key]) continue;
						
						// Check if it's an array
						if (Array.isArray(appState[key])) {
							for (let j = 0; j < appState[key].length; j++) {
								const data = appState[key][j];
								if (data && typeof data === 'string' && data.length > 50) {
									const trimmed = data.trim();
									const hasJsonPrefix = trimmed.startsWith(')]}\'');
									if (hasJsonPrefix) {
										result.foundJsonPrefix = true;
									}
									result.allStringInfo.push({
										idx: i + '.' + j,
										len: data.length,
										prefix: data.substring(0, 30),
										hasJsonPrefix: hasJsonPrefix
									});
								}
							}
						}
					}
				}
				
				return result;
			})()
		`)
		fmt.Printf("DEBUG: JS State: %+v\n", jsCheck)

		// Check if we got redirected or blocked
		if strings.Contains(strings.ToLower(title), "error") ||
		   strings.Contains(strings.ToLower(title), "sorry") ||
		   url != pageResponse.URL() {
			fmt.Printf("DEBUG: Possible block/redirect detected\n")
		}

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
	// Retry logic: when browser is cold-starting, APP_INITIALIZATION_STATE may not be fully populated yet
	// The first batch of concurrent jobs often fails because the data isn't loaded
	const maxRetries = 10
	const retryDelay = 500 * time.Millisecond

	var rawI interface{}
	var err error
	reloadAttempted := false

	for attempt := 0; attempt < maxRetries; attempt++ {
		rawI, err = page.Evaluate(js)
		if err != nil {
			return nil, err
		}

		// If we got a valid string, break out of retry loop
		if raw, ok := rawI.(string); ok && raw != "" {
			const prefix = `)]}'`
			raw = strings.TrimSpace(strings.TrimPrefix(raw, prefix))
			return []byte(raw), nil
		}

		// After a few attempts with no data, try reloading the page
		// This helps with redirected pages that didn't load data properly during browser cold-start
		if attempt == 3 && !reloadAttempted {
			reloadAttempted = true
			page.Reload(playwright.PageReloadOptions{
				WaitUntil: playwright.WaitUntilStateNetworkidle,
				Timeout:   playwright.Float(15000),
			})
			time.Sleep(1 * time.Second)
			continue
		}

		// Result was nil/empty - wait and retry
		// This happens when APP_INITIALIZATION_STATE exists but data isn't loaded yet
		if attempt < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}

	// All retries exhausted
	return nil, fmt.Errorf("could not convert to string (APP_INITIALIZATION_STATE not populated after %d retries)", maxRetries)
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
(function() {
	// Validate APP_INITIALIZATION_STATE exists and is properly structured
	if (!window.APP_INITIALIZATION_STATE) {
		return null;
	}

	if (!Array.isArray(window.APP_INITIALIZATION_STATE)) {
		return null;
	}

	// Helper function to check if a string is likely base64 encoded
	function isBase64(str) {
		if (!str || str.length < 100) return false;
		// Base64 typically contains only these characters
		const base64Regex = /^[A-Za-z0-9+/=]+$/;
		// Check first 100 chars to avoid processing entire string
		return base64Regex.test(str.substring(0, 100));
	}

	// Helper function to decode base64 and check for Google Maps data
	function tryDecodeBase64(str) {
		try {
			const decoded = atob(str);
			// Check if decoded content looks like Google Maps data
			if (decoded.startsWith(')]}\'') || decoded.includes('"title"') || decoded.includes('"address"')) {
				return decoded;
			}
		} catch(e) {}
		return null;
	}

	// Try the original index [3][6] first (for non-redirected pages)
	try {
		const appState = window.APP_INITIALIZATION_STATE[3];
		if (appState && typeof appState === 'object') {
			const keys = Object.keys(appState);
			if (keys.length > 0) {
				const key = keys[0];
				if (appState[key] && Array.isArray(appState[key]) && appState[key].length > 6) {
					const data = appState[key][6];
					if (data && typeof data === 'string' && data.length > 100) {
						// Check if it's the traditional format
						if (data.trim().startsWith(')]}\'')) {
							return data;
						}
						// Try base64 decoding
						const decoded = tryDecodeBase64(data);
						if (decoded) {
							return decoded;
						}
					}
				}
			}
		}
	} catch(e) {}

	// If that didn't work, search ALL indices and ALL positions
	let longestData = null;
	let maxLength = 0;

	for (let i = 0; i < window.APP_INITIALIZATION_STATE.length; i++) {
		const appState = window.APP_INITIALIZATION_STATE[i];

		if (!appState || typeof appState !== 'object') {
			continue;
		}

		const keys = Object.keys(appState);
		if (keys.length === 0) {
			continue;
		}

		const key = keys[0];
		if (!appState[key] || !Array.isArray(appState[key])) {
			continue;
		}

		// Search through all array elements
		for (let j = 0; j < appState[key].length; j++) {
			const data = appState[key][j];
			if (data && typeof data === 'string' && data.length > 1000) {
				const trimmed = data.trim();

				// Check for traditional Google Maps format
				if (trimmed.startsWith(')]}\'')) {
					if (data.length > maxLength) {
						longestData = data;
						maxLength = data.length;
					}
					continue;
				}

				// Check for base64 encoded data (new format)
				if (isBase64(trimmed)) {
					const decoded = tryDecodeBase64(trimmed);
					if (decoded && decoded.length > maxLength) {
						longestData = decoded;
						maxLength = decoded.length;
					}
				}
			}
		}
	}

	return longestData;
})();
`
