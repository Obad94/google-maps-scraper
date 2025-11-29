package runner

// BrowserAPI workflow implementation that combines Google Places API with browser scraping:
// Phase 1: Google Places API - Get nearby Place IDs for each category using Python script
// Phase 2: Browser Scraping - Scrape each Place URL to get full details and coordinates
// Phase 3: Nested Nearby Mode - Run nearby search at each collected location
//
// This workflow is similar to hybrid mode but uses the official Google Places API
// for initial place discovery instead of Fast/Normal/Nearby browser modes.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/scrapemateapp"
)

// placeSeed couples a category with a place URL and ID for Phase 2 scraping
type placeSeed struct {
	Category string
	PlaceID  string
	PlaceURL string
}

// runBrowserAPIPhase1 executes Phase 1: Call Google Places API to get Place IDs
// Returns place URLs and IDs for Phase 2 browser scraping
// Uses deduper to prevent scraping duplicate Place IDs
func runBrowserAPIPhase1(ctx context.Context, queries []string, cfg *Config, dedup deduper.Deduper) ([]placeSeed, error) {
	if cfg.GeoCoordinates == "" {
		return nil, fmt.Errorf("geo coordinates are required in BrowserAPI mode")
	}

	parts := strings.Split(cfg.GeoCoordinates, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid geo coordinates: %s", cfg.GeoCoordinates)
	}

	// Parse coordinates
	var lat, lng float64
	if _, err := fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &lat); err != nil {
		return nil, fmt.Errorf("invalid latitude: %w", err)
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &lng); err != nil {
		return nil, fmt.Errorf("invalid longitude: %w", err)
	}

	// Check if GOOGLE_MAPS_API_KEY is available
	if cfg.GoogleMapsAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_MAPS_API_KEY is required for BrowserAPI mode (set in .env file or environment)")
	}

	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 1: Calling Google Places API (Nearby Search)...\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Location: (%.6f, %.6f), Radius: %.0fm\n", lat, lng, cfg.Radius)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Categories: %d\n", len(queries))

	allPlaces := make([]placeSeed, 0)
	totalRequests := 0

	// For each query/category, call the Google Places API
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}

		fmt.Fprintf(os.Stderr, "[BROWSERAPI] Fetching places for category: %s\n", query)

		// Call the Google Places API Nearby Search
		places, requestCount, err := gmaps.SearchNearbyPlaces(
			ctx,
			lat,
			lng,
			cfg.Radius,
			[]string{query}, // Single category per request
			cfg.GoogleMapsAPIKey,
		)

		totalRequests += requestCount

		if err != nil {
			fmt.Fprintf(os.Stderr, "[BROWSERAPI] Error calling Places API for '%s': %v\n", query, err)
			continue // Skip this category but continue with others
		}

		fmt.Fprintf(os.Stderr, "[BROWSERAPI] Found %d places for category '%s' (%d API requests)\n",
			len(places), query, requestCount)

		// Convert API results to placeSeeds with deduplication
		duplicatesSkipped := 0
		for _, place := range places {
			if place.ID != "" {
				// Check if this Place ID has already been seen
				if dedup == nil || dedup.AddIfNotExists(ctx, place.ID) {
					allPlaces = append(allPlaces, placeSeed{
						Category: query,
						PlaceID:  place.ID,
						PlaceURL: gmaps.PlaceIDToURL(place.ID),
					})
				} else {
					duplicatesSkipped++
				}
			}
		}
		if duplicatesSkipped > 0 {
			fmt.Fprintf(os.Stderr, "[BROWSERAPI] Skipped %d duplicate Place IDs for category '%s'\n",
				duplicatesSkipped, query)
		}
	}

	estimatedCost := float64(totalRequests) * 0.017 // $0.017 per request for basic fields
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 1 complete: collected %d place URLs from %d API requests\n",
		len(allPlaces), totalRequests)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Estimated API cost: $%.4f USD\n", estimatedCost)

	return allPlaces, nil
}

// runBrowserAPIPhase2 executes Phase 2: Scrape each Place URL to get full details
// Uses PlaceJob to extract all information and coordinates
// Returns seeds for Phase 3 nested nearby search
// Deduper is passed through but deduplication already happened in Phase 1
func runBrowserAPIPhase2(ctx context.Context, places []placeSeed, cfg *Config, writers []scrapemate.ResultWriter, dedup deduper.Deduper) ([]hybridSeed, error) {
	if len(places) == 0 {
		return nil, nil
	}

	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Starting browser scraping for %d places...\n", len(places))

	exitMonitor := exiter.New()

	// Create seed collector wrapper around writers
	collectorWriter := NewSeedCollectorWriter(writers, "")

	// Create PlaceJob for each place URL
	var placeJobs []scrapemate.IJob
	for i, p := range places {
		opts := []gmaps.PlaceJobOptions{}
		if exitMonitor != nil {
			opts = append(opts, gmaps.WithPlaceJobExitMonitor(exitMonitor))
		}
		if cfg.GoogleMapsAPIKey != "" {
			opts = append(opts, gmaps.WithGoogleMapsAPIKey(cfg.GoogleMapsAPIKey))
		}

		placeJob := gmaps.NewPlaceJob(
			fmt.Sprintf("browserapi-place-%d", i),
			cfg.LangCode,
			p.PlaceURL,
			cfg.Email,
			cfg.ExtraReviews,
			opts...,
		)
		placeJobs = append(placeJobs, placeJob)
	}

	if len(placeJobs) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI] No place jobs to execute\n")
		return nil, nil
	}

	exitMonitor.SetSeedCount(len(placeJobs))

	// Configure scrapemate for browser mode
	opts := []func(*scrapemateapp.Config) error{
		scrapemateapp.WithConcurrency(cfg.Concurrency),
	}

	// Calculate smart exit-on-inactivity timeout
	// For place scraping, we don't need as much time as search scrolling
	minInactivityTimeout := 3 * time.Minute

	var exitOnInactivity time.Duration
	if cfg.ExitOnInactivityDuration > 0 {
		exitOnInactivity = cfg.ExitOnInactivityDuration
		if exitOnInactivity < minInactivityTimeout {
			fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Warning: -exit-on-inactivity %v is too short. Using minimum %v\n",
				exitOnInactivity, minInactivityTimeout)
			exitOnInactivity = minInactivityTimeout
		} else {
			fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Using exit-on-inactivity: %v\n", exitOnInactivity)
		}
	} else {
		exitOnInactivity = minInactivityTimeout * 2
		fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Using smart default exit-on-inactivity: %v\n", exitOnInactivity)
	}
	opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))

	if cfg.Debug {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
	} else {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
	}
	if len(cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
	}
	if !cfg.DisablePageReuse {
		opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithBrowserReuseLimit(200))
	}

	// Use the collector writer instead of original writers
	matecfg, err := scrapemateapp.NewConfig([]scrapemate.ResultWriter{collectorWriter}, opts...)
	if err != nil {
		return nil, err
	}
	app, err := scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return nil, err
	}
	defer app.Close()

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	exitMonitor.SetCancelFunc(cancel)
	go exitMonitor.Run(ctx2)

	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Executing %d Place scraping jobs...\n", len(placeJobs))
	if err := app.Start(ctx2, placeJobs...); err != nil {
		if !errors.Is(err, context.Canceled) {
			return nil, err
		}
	}

	// Collect seeds from the writer (results are already written by underlying writers)
	seeds := collectorWriter.GetSeeds()
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Place scraping completed, collected %d seeds\n", len(seeds))

	return seeds, nil
}

// RunBrowserAPIFile executes the BrowserAPI workflow for the file runner.
// Phases:
// 1. Google Places API - Get nearby Place IDs for each category
// 2. Browser Scraping - Scrape each Place URL to get full details
// 3. Nested Nearby Mode - Run nearby search at each collected location
func RunBrowserAPIFile(ctx context.Context, cfg *Config, input io.Reader, writers []scrapemate.ResultWriter) error {
	// Gather queries from input reader
	scanner := bufio.NewScanner(input)
	queries := []string{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			if before, after, ok := strings.Cut(line, "#!#"); ok {
				line = strings.TrimSpace(before)
				/* input id ignored */
				_ = after
			}
			queries = append(queries, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(queries) == 0 {
		return fmt.Errorf("no queries provided for BrowserAPI mode")
	}

	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] BrowserAPI Mode Starting\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Queries: %d\n", len(queries))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Geo: %s, Radius: %.0fm\n", cfg.GeoCoordinates, cfg.Radius)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Depth: %d, Zoom: %d\n", cfg.MaxDepth, cfg.ZoomLevel)
	fmt.Fprintf(os.Stderr, "========================================\n\n")

	// Create shared deduper for all phases to prevent duplicate scraping
	dedup := deduper.New()

	// Phase 1: Call Google Places API to get Place IDs (with deduplication)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 1: Fetching places from Google Places API...\n")
	placeSeeds, err := runBrowserAPIPhase1(ctx, queries, cfg, dedup)
	if err != nil {
		return fmt.Errorf("Phase 1 (Google Places API) failed: %w", err)
	}
	if len(placeSeeds) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI] No places found from API. Exiting.\n")
		return nil
	}
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 1 complete: collected %d place URLs\n\n", len(placeSeeds))

	// Phase 2: Scrape each Place URL to get full details and coordinates
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2: Scraping place details...\n")
	locationSeeds, err := runBrowserAPIPhase2(ctx, placeSeeds, cfg, writers, dedup)
	if err != nil {
		return fmt.Errorf("Phase 2 (Place Scraping) failed: %w", err)
	}
	if len(locationSeeds) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI] No locations extracted from scraped places. Exiting.\n")
		return nil
	}
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2 complete: collected %d location seeds\n\n", len(locationSeeds))

	// Phase 3: Nested Nearby Mode on all collected locations
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 2 Summary:\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI]   Places from API:       %d\n", len(placeSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI]   Locations scraped:     %d\n", len(locationSeeds))
	fmt.Fprintf(os.Stderr, "========================================\n\n")

	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Phase 3: Creating Nested Nearby Mode jobs for all %d locations...\n", len(locationSeeds))
	// Reuse the same deduper from Phase 1 & 2 to prevent re-scraping already-seen places
	exitMonitor := exiter.New()
	nestedNearbyJobs := buildHybridNearbyJobs(locationSeeds, cfg, dedup, exitMonitor)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Created %d nested nearby jobs\n", len(nestedNearbyJobs))

	if len(nestedNearbyJobs) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI] No nested nearby jobs to execute. Finished.\n")
		return nil
	}
	exitMonitor.SetSeedCount(len(nestedNearbyJobs))

	opts := []func(*scrapemateapp.Config) error{scrapemateapp.WithConcurrency(cfg.Concurrency)}

	// Apply exit-on-inactivity if configured
	if cfg.ExitOnInactivityDuration > 0 {
		minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute
		exitOnInactivity := cfg.ExitOnInactivityDuration
		if exitOnInactivity < minInactivityTimeout {
			fmt.Fprintf(os.Stderr, "[BROWSERAPI] Warning: -exit-on-inactivity %v is too short for depth %d. Using minimum %v\n",
				exitOnInactivity, cfg.MaxDepth, minInactivityTimeout)
			exitOnInactivity = minInactivityTimeout
		}
		opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
	}

	// Browser / JS for nearby jobs
	if cfg.Debug {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
	} else {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
	}
	if len(cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
	}
	if !cfg.DisablePageReuse {
		opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithBrowserReuseLimit(200))
	}

	matecfg, err := scrapemateapp.NewConfig(writers, opts...)
	if err != nil {
		return err
	}
	app, err := scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	exitMonitor.SetCancelFunc(cancel)
	go exitMonitor.Run(ctx2)

	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Executing Phase 3: %d nested nearby jobs...\n", len(nestedNearbyJobs))
	if err := app.Start(ctx2, nestedNearbyJobs...); err != nil {
		// Context cancellation is expected when exiter completes all jobs - not an error
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "\n========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] BrowserAPI Mode Complete!\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Places from API: %d\n", len(placeSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Locations scraped: %d\n", len(locationSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI] Nested nearby jobs executed: %d\n", len(nestedNearbyJobs))
	fmt.Fprintf(os.Stderr, "========================================\n")

	Telemetry().Send(ctx, tlmt.NewEvent("browserapi_runner", map[string]any{
		"places_from_api":    len(placeSeeds),
		"locations_scraped":  len(locationSeeds),
		"nested_nearby_jobs": len(nestedNearbyJobs),
	}))

	return nil
}

// RunBrowserAPIWeb executes the BrowserAPI workflow for the web runner.
// Mirrors RunBrowserAPIFile but accepts pre-parsed keyword slice and writers.
func RunBrowserAPIWeb(ctx context.Context, cfg *Config, keywords []string, writers []scrapemate.ResultWriter) error {
	if len(keywords) == 0 {
		return fmt.Errorf("no keywords provided for BrowserAPI mode")
	}

	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] BrowserAPI Mode Starting\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Keywords: %d\n", len(keywords))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Geo: %s, Radius: %.0fm\n", cfg.GeoCoordinates, cfg.Radius)
	fmt.Fprintf(os.Stderr, "========================================\n\n")

	// Create shared deduper for all phases to prevent duplicate scraping
	dedup := deduper.New()

	// Phase 1: Call Google Places API to get Place IDs (with deduplication)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 1: Fetching places from Google Places API...\n")
	placeSeeds, err := runBrowserAPIPhase1(ctx, keywords, cfg, dedup)
	if err != nil {
		return fmt.Errorf("Phase 1 (Google Places API) failed: %w", err)
	}
	if len(placeSeeds) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] No places found from API. Exiting.\n")
		return nil
	}
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 1 complete: collected %d place URLs\n\n", len(placeSeeds))

	// Phase 2: Scrape each Place URL to get full details and coordinates
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 2: Scraping place details...\n")
	locationSeeds, err := runBrowserAPIPhase2(ctx, placeSeeds, cfg, writers, dedup)
	if err != nil {
		return fmt.Errorf("Phase 2 (Place Scraping) failed: %w", err)
	}
	if len(locationSeeds) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] No locations extracted from scraped places. Exiting.\n")
		return nil
	}
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 2 complete: collected %d location seeds\n\n", len(locationSeeds))

	// Phase 3: Nested Nearby Mode on all collected locations
	fmt.Fprintf(os.Stderr, "========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 2 Summary:\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB]   Places from API:       %d\n", len(placeSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB]   Locations scraped:     %d\n", len(locationSeeds))
	fmt.Fprintf(os.Stderr, "========================================\n\n")

	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Phase 3: Creating Nested Nearby Mode jobs for all %d locations...\n", len(locationSeeds))
	// Reuse the same deduper from Phase 1 & 2 to prevent re-scraping already-seen places
	exitMonitor := exiter.New()
	nestedNearbyJobs := buildHybridNearbyJobs(locationSeeds, cfg, dedup, exitMonitor)
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Created %d nested nearby jobs\n", len(nestedNearbyJobs))

	if len(nestedNearbyJobs) == 0 {
		fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] No nested nearby jobs to execute. Finished.\n")
		return nil
	}
	exitMonitor.SetSeedCount(len(nestedNearbyJobs))

	opts := []func(*scrapemateapp.Config) error{scrapemateapp.WithConcurrency(cfg.Concurrency)}

	// Calculate smart exit-on-inactivity timeout
	minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute

	var exitOnInactivity time.Duration
	if cfg.ExitOnInactivityDuration > 0 {
		exitOnInactivity = cfg.ExitOnInactivityDuration
		if exitOnInactivity < minInactivityTimeout {
			exitOnInactivity = minInactivityTimeout
		}
	} else {
		exitOnInactivity = minInactivityTimeout * 2
	}
	opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))

	if cfg.Debug {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
	} else {
		opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
	}
	if len(cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
	}
	if !cfg.DisablePageReuse {
		opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithBrowserReuseLimit(200))
	}

	matecfg, err := scrapemateapp.NewConfig(writers, opts...)
	if err != nil {
		return err
	}
	app, err := scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return err
	}
	defer app.Close()

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	exitMonitor.SetCancelFunc(cancel)
	go exitMonitor.Run(ctx2)

	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Executing Phase 3: %d nested nearby jobs...\n", len(nestedNearbyJobs))
	if err := app.Start(ctx2, nestedNearbyJobs...); err != nil {
		// Context cancellation is expected when exiter completes all jobs - not an error
		if !errors.Is(err, context.Canceled) {
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "\n========================================\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] BrowserAPI Mode Complete!\n")
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Places from API: %d\n", len(placeSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Locations scraped: %d\n", len(locationSeeds))
	fmt.Fprintf(os.Stderr, "[BROWSERAPI-WEB] Nested nearby jobs executed: %d\n", len(nestedNearbyJobs))
	fmt.Fprintf(os.Stderr, "========================================\n")

	return nil
}
