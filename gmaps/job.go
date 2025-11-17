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

type GmapJobOptions func(*GmapJob)

type GmapJob struct {
	scrapemate.Job

	MaxDepth     int
	LangCode     string
	ExtractEmail bool

	Deduper             deduper.Deduper
	ExitMonitor         exiter.Exiter
	ExtractExtraReviews bool

	// Radius filtering
	FilterByRadius bool
	CenterLat      float64
	CenterLon      float64
	RadiusMeters   float64
}

func NewGmapJob(
	id, langCode, query string,
	maxDepth int,
	extractEmail bool,
	geoCoordinates string,
	zoom int,
	opts ...GmapJobOptions,
) *GmapJob {
	query = url.QueryEscape(query)

	const (
		maxRetries = 3
		prio       = scrapemate.PriorityLow
	)

	if id == "" {
		id = uuid.New().String()
	}

	mapURL := ""
	if geoCoordinates != "" && zoom > 0 {
		mapURL = fmt.Sprintf("https://www.google.com/maps/search/%s/@%s,%dz", query, strings.ReplaceAll(geoCoordinates, " ", ""), zoom)
	} else {
		//Warning: geo and zoom MUST be both set or not
		mapURL = fmt.Sprintf("https://www.google.com/maps/search/%s", query)
	}

	job := GmapJob{
		Job: scrapemate.Job{
			ID:         id,
			Method:     http.MethodGet,
			URL:        mapURL,
			URLParams:  map[string]string{"hl": langCode},
			MaxRetries: maxRetries,
			Priority:   prio,
		},
		MaxDepth:     maxDepth,
		LangCode:     langCode,
		ExtractEmail: extractEmail,
	}

	for _, opt := range opts {
		opt(&job)
	}

	return &job
}

func WithDeduper(d deduper.Deduper) GmapJobOptions {
	return func(j *GmapJob) {
		j.Deduper = d
	}
}

func WithExitMonitor(e exiter.Exiter) GmapJobOptions {
	return func(j *GmapJob) {
		j.ExitMonitor = e
	}
}

func WithExtraReviews() GmapJobOptions {
	return func(j *GmapJob) {
		j.ExtractExtraReviews = true
	}
}

func WithRadiusFiltering(lat, lon, radiusMeters float64) GmapJobOptions {
	return func(j *GmapJob) {
		j.FilterByRadius = true
		j.CenterLat = lat
		j.CenterLon = lon
		j.RadiusMeters = radiusMeters
	}
}

func (j *GmapJob) UseInResults() bool {
	return false
}

func (j *GmapJob) Process(ctx context.Context, resp *scrapemate.Response) (any, []scrapemate.IJob, error) {
	defer func() {
		resp.Document = nil
		resp.Body = nil
	}()

	log := scrapemate.GetLoggerFromContext(ctx)

	doc, ok := resp.Document.(*goquery.Document)
	if !ok {
		return nil, nil, fmt.Errorf("could not convert to goquery document")
	}

	var next []scrapemate.IJob

	if strings.Contains(resp.URL, "/maps/place/") {
		jopts := []PlaceJobOptions{}
		if j.ExitMonitor != nil {
			jopts = append(jopts, WithPlaceJobExitMonitor(j.ExitMonitor))
		}
		if j.FilterByRadius {
			jopts = append(jopts, WithRadiusFilter(j.CenterLat, j.CenterLon, j.RadiusMeters))
		}

		placeJob := NewPlaceJob(j.ID, j.LangCode, resp.URL, j.ExtractEmail, j.ExtractExtraReviews, jopts...)

		next = append(next, placeJob)
	} else {
		doc.Find(`div[role=feed] div[jsaction]>a`).Each(func(_ int, s *goquery.Selection) {
			if href := s.AttrOr("href", ""); href != "" {
				jopts := []PlaceJobOptions{}
				if j.ExitMonitor != nil {
					jopts = append(jopts, WithPlaceJobExitMonitor(j.ExitMonitor))
				}
				if j.FilterByRadius {
					jopts = append(jopts, WithRadiusFilter(j.CenterLat, j.CenterLon, j.RadiusMeters))
				}

				nextJob := NewPlaceJob(j.ID, j.LangCode, href, j.ExtractEmail, j.ExtractExtraReviews, jopts...)

				if j.Deduper == nil || j.Deduper.AddIfNotExists(ctx, href) {
					next = append(next, nextJob)
				}
			}
		})
	}

	if j.ExitMonitor != nil {
		j.ExitMonitor.IncrPlacesFound(len(next))
		j.ExitMonitor.IncrSeedCompleted(1)
	}

	log.Info(fmt.Sprintf("%d places found", len(next)))

	return nil, next, nil
}

func (j *GmapJob) BrowserActions(ctx context.Context, page playwright.Page) scrapemate.Response {
	var resp scrapemate.Response

	pageResponse, err := page.Goto(j.GetFullURL(), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
	})

	if err != nil {
		resp.Error = err

		return resp
	}

	if err = clickRejectCookiesIfRequired(page); err != nil {
		resp.Error = err

		return resp
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

	resp.URL = pageResponse.URL()
	resp.StatusCode = pageResponse.Status()
	resp.Headers = make(http.Header, len(pageResponse.Headers()))

	for k, v := range pageResponse.Headers() {
		resp.Headers.Add(k, v)
	}

	// When Google Maps finds only 1 place, it slowly redirects to that place's URL
	// check element scroll
	sel := `div[role='feed']`

	//nolint:staticcheck // TODO replace with the new playwright API
	_, err = page.WaitForSelector(sel, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(700),
	})

	var singlePlace bool

	if err != nil {
		waitCtx, waitCancel := context.WithTimeout(ctx, time.Second*5)
		defer waitCancel()

		singlePlace = waitUntilURLContains(waitCtx, page, "/maps/place/")

		waitCancel()
	}

	if singlePlace {
		resp.URL = page.URL()

		var body string

		body, err = page.Content()
		if err != nil {
			resp.Error = err
			return resp
		}

		resp.Body = []byte(body)

		return resp
	}

	scrollSelector := `div[role='feed']`

	_, err = scroll(ctx, page, j.MaxDepth, scrollSelector)
	if err != nil {
		resp.Error = err

		return resp
	}

	body, err := page.Content()
	if err != nil {
		resp.Error = err
		return resp
	}

	resp.Body = []byte(body)

	return resp
}

func waitUntilURLContains(ctx context.Context, page playwright.Page, s string) bool {
	ticker := time.NewTicker(time.Millisecond * 150)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if strings.Contains(page.URL(), s) {
				return true
			}
		}
	}
}

func clickRejectCookiesIfRequired(page playwright.Page) error {
	// click the cookie reject button if exists
	sel := `form[action="https://consent.google.com/save"]:first-of-type button:first-of-type`

	const timeout = 500

	//nolint:staticcheck // TODO replace with the new playwright API
	el, err := page.WaitForSelector(sel, playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(timeout),
	})

	if err != nil {
		return nil
	}

	if el == nil {
		return nil
	}

	//nolint:staticcheck // TODO replace with the new playwright API
	return el.Click()
}

func scroll(ctx context.Context,
	page playwright.Page,
	maxDepth int,
	scrollSelector string,
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
	var stableCount int // Count of consecutive times height hasn't changed
	scrollAttempts := 0

	const (
		initialWait       = 800  // Initial wait time for content to load (ms)
		maxWait           = 60000 // Maximum wait time between scrolls (ms)
		minWait           = 500  // Minimum wait time between scrolls (ms)
		stableThreshold   = 5    // Number of stable checks before considering content fully loaded
		retryWaitIncrease = 500  // Additional wait time when content appears stable
	)

	// Progressive wait time - starts at initialWait, increases gradually
	waitTime := float64(initialWait)

	for i := 0; i < maxDepth; i++ {
		scrollAttempts++

		// Calculate wait time for this scroll iteration
		// If content was stable last time, wait longer to give it more time to load
		currentWaitTime := int(waitTime)
		if stableCount > 0 {
			currentWaitTime += retryWaitIncrease * stableCount
		}
		if currentWaitTime > maxWait {
			currentWaitTime = maxWait
		}
		if currentWaitTime < minWait {
			currentWaitTime = minWait
		}

		// Scroll to the bottom and wait for content to load
		scrollHeight, err := page.Evaluate(fmt.Sprintf(expr, currentWaitTime))
		if err != nil {
			return scrollAttempts, err
		}

		height, ok := scrollHeight.(int)
		if !ok {
			// Element not found or invalid height
			if height, ok := scrollHeight.(float64); ok && height == -1 {
				return scrollAttempts, fmt.Errorf("scroll element %q not found", scrollSelector)
			}
			return scrollAttempts, fmt.Errorf("scrollHeight is not an int: %v", scrollHeight)
		}

		// Check if height has changed (new content loaded)
		if height == currentScrollHeight {
			stableCount++

			// Only exit early if we've seen stable height multiple times
			// This ensures we don't exit prematurely due to slow network
			if stableCount >= stableThreshold {
				// Content appears fully loaded - no more items
				break
			}

			// Content might still be loading, continue with increased wait time
			waitTime += float64(retryWaitIncrease)
		} else {
			// New content loaded - reset stable count
			stableCount = 0
			currentScrollHeight = height

			// Gradually increase wait time for subsequent scrolls
			waitTime *= 1.3
		}

		// Respect context cancellation
		select {
		case <-ctx.Done():
			return scrollAttempts, nil
		default:
		}

		// Cap the wait time
		if waitTime > float64(maxWait) {
			waitTime = float64(maxWait)
		}
		if waitTime < float64(minWait) {
			waitTime = float64(minWait)
		}

		// Additional small wait between scroll iterations to ensure smooth scrolling
		//nolint:staticcheck // TODO replace with the new playwright API
		page.WaitForTimeout(200)
	}

	return scrollAttempts, nil
}
