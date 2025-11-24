package runner

// Extended Hybrid workflow implementation that combines three search modes:
// Phase 1a: Fast Mode (HTTP API) - quick, ~21 results per query
// Phase 1b: Normal Mode (Browser) - comprehensive browser-based search
// Phase 1c: Initial Nearby Mode (Browser) - proximity search at input coordinates
// Phase 2: Nested Nearby Mode (Browser) - nearby search at all found locations
//
// Rationale:
// - Scrapemate cannot mix stealth/HTTP and browser/JS modes reliably in a
//   single run (reported breakage of Fast Mode when both enabled).
// - We therefore execute Fast Mode outside scrapemate using HTTP API.
// - Normal Mode and Initial Nearby Mode run in first browser session.
// - All results from Phase 1 (a, b, c) are collected with their coordinates.
// - Phase 2 creates NearbySearchJobs for every location found in Phase 1.
// - All results (Phase 1 + Phase 2) are written to CSV for maximum coverage.
//
// This file adds helper functions used by both the file runner and the
// web runner when -hybrid-mode is enabled.

import (
    "bufio"
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "math"
    "net/http"
    "net/url"
    "os"
    "strconv"
    "strings"
    "sync"
    "time"

    "github.com/gosom/google-maps-scraper/deduper"
    "github.com/gosom/google-maps-scraper/exiter"
    "github.com/gosom/google-maps-scraper/gmaps"
    "github.com/gosom/google-maps-scraper/tlmt"
    "github.com/gosom/scrapemate"
    "github.com/gosom/scrapemate/scrapemateapp"
)

// haversineDistance calculates the distance between two coordinates in meters
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
    const earthRadiusMeters = 6371000.0
    dLat := (lat2 - lat1) * math.Pi / 180.0
    dLon := (lon2 - lon1) * math.Pi / 180.0
    lat1Rad := lat1 * math.Pi / 180.0
    lat2Rad := lat2 * math.Pi / 180.0
    a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dLon/2)*math.Sin(dLon/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    return earthRadiusMeters * c
}

// hybridSeed couples an originating query with a fast mode entry.
type hybridSeed struct {
    Query string
    Entry *gmaps.Entry
}

// SeedCollectorWriter wraps ResultWriters and collects coordinates from results
// for Phase 2 nested nearby search.
type SeedCollectorWriter struct {
    Writers []scrapemate.ResultWriter
    Seeds   []hybridSeed
    Query   string
    mu      sync.Mutex
}

func NewSeedCollectorWriter(writers []scrapemate.ResultWriter, query string) *SeedCollectorWriter {
    return &SeedCollectorWriter{
        Writers: writers,
        Query:   query,
        Seeds:   make([]hybridSeed, 0),
    }
}

// Run implements scrapemate.ResultWriter interface
func (w *SeedCollectorWriter) Run(ctx context.Context, in <-chan scrapemate.Result) error {
    // Create channels for each underlying writer
    writerChannels := make([]chan scrapemate.Result, len(w.Writers))
    for i := range w.Writers {
        writerChannels[i] = make(chan scrapemate.Result, 100)
    }

    // Start all underlying writers in goroutines
    errChan := make(chan error, len(w.Writers))
    var wg sync.WaitGroup

    for i, writer := range w.Writers {
        wg.Add(1)
        go func(w scrapemate.ResultWriter, ch chan scrapemate.Result) {
            defer wg.Done()
            if err := w.Run(ctx, ch); err != nil {
                errChan <- err
            }
        }(writer, writerChannels[i])
    }

    // Process results from input channel
    go func() {
        for result := range in {
            // Extract coordinates if this is an Entry
            if result.Data != nil {
                // Handle both single entry and slice of entries
                if entry, ok := result.Data.(*gmaps.Entry); ok {
                    if entry.Latitude != 0 || entry.Longtitude != 0 {
                        w.mu.Lock()
                        w.Seeds = append(w.Seeds, hybridSeed{
                            Query: w.Query,
                            Entry: entry,
                        })
                        w.mu.Unlock()
                    }
                } else if entries, ok := result.Data.([]*gmaps.Entry); ok {
                    for _, entry := range entries {
                        if entry.Latitude != 0 || entry.Longtitude != 0 {
                            w.mu.Lock()
                            w.Seeds = append(w.Seeds, hybridSeed{
                                Query: w.Query,
                                Entry: entry,
                            })
                            w.mu.Unlock()
                        }
                    }
                }
            }

            // Forward result to all underlying writers
            for _, ch := range writerChannels {
                select {
                case ch <- result:
                case <-ctx.Done():
                    return
                }
            }
        }

        // Close all writer channels when input is done
        for _, ch := range writerChannels {
            close(ch)
        }
    }()

    // Wait for all writers to complete
    wg.Wait()
    close(errChan)

    // Check for errors
    for err := range errChan {
        if err != nil {
            return err
        }
    }

    return nil
}

func (w *SeedCollectorWriter) GetSeeds() []hybridSeed {
    w.mu.Lock()
    defer w.mu.Unlock()
    seeds := make([]hybridSeed, len(w.Seeds))
    copy(seeds, w.Seeds)
    return seeds
}

// runHybridFastPhase executes Phase 1a: Fast Mode API (HTTP) for each query.
// Returns up to ~21 results per query ordered by distance.
// These results are seeds for Phase 2 nested nearby search.
func runHybridFastPhase(ctx context.Context, queries []string, lang string, geo string, zoom int, radius float64) ([]hybridSeed, error) {
    if geo == "" {
        return nil, fmt.Errorf("geo coordinates are required in hybrid mode")
    }
    parts := strings.Split(geo, ",")
    if len(parts) != 2 {
        return nil, fmt.Errorf("invalid geo coordinates: %s", geo)
    }
    lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
    if err != nil { return nil, fmt.Errorf("invalid latitude: %w", err) }
    lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
    if err != nil { return nil, fmt.Errorf("invalid longitude: %w", err) }
    if zoom < 1 || zoom > 21 { return nil, fmt.Errorf("invalid zoom level: %d", zoom) }
    if radius < 0 { return nil, fmt.Errorf("invalid radius: %f", radius) }

    client := &http.Client{ Timeout: 20 * time.Second }
    allSeeds := make([]hybridSeed, 0, len(queries)*20)

    for _, q := range queries {
        q = strings.TrimSpace(q)
        if q == "" { continue }

        params := &gmaps.MapSearchParams{
            Location: gmaps.MapLocation{ Lat: lat, Lon: lon, ZoomLvl: float64(zoom), Radius: radius },
            Query: q,
            ViewportW: 1920,
            ViewportH: 450,
            Hl: lang,
        }
        sj := gmaps.NewSearchJob(params)

        // Build request URL with params
        u, err := url.Parse(sj.URL)
        if err != nil { return nil, err }
        qv := u.Query()
        for k,v := range sj.URLParams { qv.Set(k,v) }
        u.RawQuery = qv.Encode()

        req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
        if err != nil { return nil, err }
        req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36")

        resp, err := client.Do(req)
        if err != nil { return nil, fmt.Errorf("fast mode request failed for '%s': %w", q, err) }
        body, err := io.ReadAll(resp.Body)
        _ = resp.Body.Close()
        if err != nil { return nil, err }

        // Parse results directly to get visibility into pre-filter vs post-filter counts
        // Remove first line (it's not JSON) before parsing
        if len(body) > 0 {
            if idx := bytes.IndexByte(body, '\n'); idx >= 0 {
                body = body[idx+1:]
            }
        }

        allEntries, perr := gmaps.ParseSearchResults(body)
        if perr != nil { return nil, fmt.Errorf("fast mode parse failed for '%s': %w", q, perr) }

        fmt.Fprintf(os.Stderr, "[HYBRID] Fast mode API returned %d results for query '%s'\n", len(allEntries), q)

        // Apply radius filtering if radius > 0
        var entries []*gmaps.Entry
        if radius > 0 {
            for _, e := range allEntries {
                dist := haversineDistance(lat, lon, e.Latitude, e.Longtitude)
                if dist <= radius {
                    entries = append(entries, e)
                }
            }
            fmt.Fprintf(os.Stderr, "[HYBRID] After radius filter (%.0fm): %d results remain\n", radius, len(entries))
        } else {
            entries = allEntries
        }

        for _, e := range entries {
            allSeeds = append(allSeeds, hybridSeed{Query: q, Entry: e})
        }
    }
    return allSeeds, nil
}

// runHybridNormalPhase executes Phase 1b: Normal Mode (Browser) for each query.
// Returns results from browser-based search with scrolling.
// These results are seeds for Phase 2 nested nearby search.
func runHybridNormalPhase(ctx context.Context, queries []string, cfg *Config, writers []scrapemate.ResultWriter) ([]hybridSeed, error) {
    if cfg.GeoCoordinates == "" {
        return nil, fmt.Errorf("geo coordinates are required in hybrid mode")
    }

    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1b: Starting Normal Mode for %d queries...\n", len(queries))

    dedup := deduper.New()
    exitMonitor := exiter.New()

    // Create seed collector wrapper around writers
    // Use empty string for Query - Phase 2 will use Entry.Category as fallback
    collectorWriter := NewSeedCollectorWriter(writers, "")

    // Create Normal Mode jobs for each query
    var normalJobs []scrapemate.IJob
    for i, q := range queries {
        q = strings.TrimSpace(q)
        if q == "" {
            continue
        }

        opts := []gmaps.GmapJobOptions{}
        if dedup != nil {
            opts = append(opts, gmaps.WithDeduper(dedup))
        }
        if exitMonitor != nil {
            opts = append(opts, gmaps.WithExitMonitor(exitMonitor))
        }
        if cfg.ExtraReviews {
            opts = append(opts, gmaps.WithExtraReviews())
        }
        if cfg.Radius > 0 {
            parts := strings.Split(cfg.GeoCoordinates, ",")
            if len(parts) == 2 {
                lat, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
                lon, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
                opts = append(opts, gmaps.WithRadiusFiltering(lat, lon, cfg.Radius))
            }
        }
        if cfg.GoogleMapsAPIKey != "" {
            opts = append(opts, gmaps.WithGmapGoogleMapsAPIKey(cfg.GoogleMapsAPIKey))
        }

        normalJob := gmaps.NewGmapJob(
            fmt.Sprintf("hybrid-normal-%d", i),
            cfg.LangCode,
            q,
            cfg.MaxDepth,
            cfg.Email,
            cfg.GeoCoordinates,
            cfg.ZoomLevel,
            opts...,
        )
        normalJobs = append(normalJobs, normalJob)
    }

    if len(normalJobs) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID] No normal mode jobs to execute\n")
        return nil, nil
    }

    exitMonitor.SetSeedCount(len(normalJobs))

    // Configure scrapemate for browser mode
    opts := []func(*scrapemateapp.Config) error{
        scrapemateapp.WithConcurrency(cfg.Concurrency),
    }

    if cfg.ExitOnInactivityDuration > 0 {
        minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute
        exitOnInactivity := cfg.ExitOnInactivityDuration
        if exitOnInactivity < minInactivityTimeout {
            exitOnInactivity = minInactivityTimeout
        }
        opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
    }

    if cfg.Debug {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
    } else {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
    }
    if len(cfg.Proxies) > 0 {
        opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
    }
    if !cfg.DisablePageReuse {
        opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithPageReuseLimit(200))
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

    fmt.Fprintf(os.Stderr, "[HYBRID] Executing %d Normal Mode jobs...\n", len(normalJobs))
    if err := app.Start(ctx2, normalJobs...); err != nil {
        if !errors.Is(err, context.Canceled) {
            return nil, err
        }
    }

    // Collect seeds from the writer (results are already written by underlying writers)
    seeds := collectorWriter.GetSeeds()
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1b: Normal Mode completed, collected %d seeds\n", len(seeds))

    return seeds, nil
}

// runHybridInitialNearbyPhase executes Phase 1c: Initial Nearby Mode at input coordinates.
// Returns results from proximity search at the center point.
// These results are seeds for Phase 2 nested nearby search.
func runHybridInitialNearbyPhase(ctx context.Context, queries []string, cfg *Config, writers []scrapemate.ResultWriter) ([]hybridSeed, error) {
    if cfg.GeoCoordinates == "" {
        return nil, fmt.Errorf("geo coordinates are required in hybrid mode")
    }

    parts := strings.Split(cfg.GeoCoordinates, ",")
    if len(parts) != 2 {
        return nil, fmt.Errorf("invalid geo coordinates: %s", cfg.GeoCoordinates)
    }
    lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
    if err != nil {
        return nil, fmt.Errorf("invalid latitude: %w", err)
    }
    lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
    if err != nil {
        return nil, fmt.Errorf("invalid longitude: %w", err)
    }

    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1c: Starting Initial Nearby Mode at (%.6f, %.6f) for %d categories...\n", lat, lon, len(queries))

    dedup := deduper.New()
    exitMonitor := exiter.New()
    zoomMeters := ConvertZoomToMeters(cfg.ZoomLevel, lat)

    // Create seed collector wrapper around writers
    // Use empty string for Query - Phase 2 will use Entry.Category as fallback
    collectorWriter := NewSeedCollectorWriter(writers, "")

    // Create Initial Nearby Mode jobs for each query/category
    var nearbyJobs []scrapemate.IJob
    for i, q := range queries {
        q = strings.TrimSpace(q)
        if q == "" {
            continue
        }

        opts := []gmaps.NearbySearchJobOptions{}
        if dedup != nil {
            opts = append(opts, gmaps.WithNearbyDeduper(dedup))
        }
        if exitMonitor != nil {
            opts = append(opts, gmaps.WithNearbyExitMonitor(exitMonitor))
        }
        if cfg.ExtraReviews {
            opts = append(opts, gmaps.WithNearbyExtraReviews())
        }
        if cfg.Radius > 0 {
            opts = append(opts, gmaps.WithNearbyRadiusFiltering(lat, lon, cfg.Radius))
        }
        if zoomMeters > 0 {
            opts = append(opts, gmaps.WithNearbyZoom(float64(zoomMeters)))
        }
        if cfg.GoogleMapsAPIKey != "" {
            opts = append(opts, gmaps.WithNearbyGoogleMapsAPIKey(cfg.GoogleMapsAPIKey))
        }

        nearbyJob := gmaps.NewNearbySearchJob(
            fmt.Sprintf("hybrid-initial-nearby-%d", i),
            cfg.LangCode,
            lat,
            lon,
            q,
            cfg.MaxDepth,
            cfg.Email,
            opts...,
        )
        nearbyJobs = append(nearbyJobs, nearbyJob)
    }

    if len(nearbyJobs) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID] No initial nearby mode jobs to execute\n")
        return nil, nil
    }

    exitMonitor.SetSeedCount(len(nearbyJobs))

    // Configure scrapemate for browser mode
    opts := []func(*scrapemateapp.Config) error{
        scrapemateapp.WithConcurrency(cfg.Concurrency),
    }

    if cfg.ExitOnInactivityDuration > 0 {
        minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute
        exitOnInactivity := cfg.ExitOnInactivityDuration
        if exitOnInactivity < minInactivityTimeout {
            exitOnInactivity = minInactivityTimeout
        }
        opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
    }

    if cfg.Debug {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
    } else {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
    }
    if len(cfg.Proxies) > 0 {
        opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
    }
    if !cfg.DisablePageReuse {
        opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithPageReuseLimit(200))
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

    fmt.Fprintf(os.Stderr, "[HYBRID] Executing %d Initial Nearby Mode jobs...\n", len(nearbyJobs))
    if err := app.Start(ctx2, nearbyJobs...); err != nil {
        if !errors.Is(err, context.Canceled) {
            return nil, err
        }
    }

    // Collect seeds from the writer (results are already written by underlying writers)
    seeds := collectorWriter.GetSeeds()
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1c: Initial Nearby Mode completed, collected %d seeds\n", len(seeds))

    return seeds, nil
}

// buildHybridNearbyJobs creates a NearbySearchJob for every seed entry, using the
// original query string as the category to trigger proper feed loading.
func buildHybridNearbyJobs(seeds []hybridSeed, cfg *Config, dedup deduper.Deduper, exitMonitor exiter.Exiter) []scrapemate.IJob {
    jobs := make([]scrapemate.IJob, 0, len(seeds))
    zoomMeters := ConvertZoomToMeters(cfg.ZoomLevel, func() float64 { if len(seeds)>0 { return seeds[0].Entry.Latitude } ; return 0 }())
    for i, s := range seeds {
        if s.Entry == nil { continue }
        // Use originating query; if blank fallback to entry.Category, then to generic "business".
        category := strings.TrimSpace(s.Query)
        if category == "" { category = strings.TrimSpace(s.Entry.Category) }
        if category == "" { category = "business" }

        opts := []gmaps.NearbySearchJobOptions{}
        if dedup != nil { opts = append(opts, gmaps.WithNearbyDeduper(dedup)) }
        if exitMonitor != nil { opts = append(opts, gmaps.WithNearbyExitMonitor(exitMonitor)) }
        if cfg.ExtraReviews { opts = append(opts, gmaps.WithNearbyExtraReviews()) }
        if cfg.Radius > 0 { opts = append(opts, gmaps.WithNearbyRadiusFiltering(s.Entry.Latitude, s.Entry.Longtitude, cfg.Radius)) }
        if zoomMeters > 0 { opts = append(opts, gmaps.WithNearbyZoom(float64(zoomMeters))) }
        if cfg.GoogleMapsAPIKey != "" { opts = append(opts, gmaps.WithNearbyGoogleMapsAPIKey(cfg.GoogleMapsAPIKey)) }

        nj := gmaps.NewNearbySearchJob(fmt.Sprintf("hybrid-nearby-%d", i), cfg.LangCode, s.Entry.Latitude, s.Entry.Longtitude, category, cfg.MaxDepth, cfg.Email, opts...)
        jobs = append(jobs, nj)
    }
    return jobs
}

// RunHybridFile executes the extended hybrid workflow for the file runner.
// Phases:
// 1a. Fast Mode (HTTP) on input parameters
// 1b. Normal Mode (Browser) on input parameters
// 1c. Initial Nearby Mode (Browser) on input parameters
// 2. Nested Nearby Mode (Browser) on all collected seed locations
func RunHybridFile(ctx context.Context, cfg *Config, input io.Reader, writers []scrapemate.ResultWriter) error {
    // Gather queries from input reader
    scanner := bufio.NewScanner(input)
    queries := []string{}
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line != "" {
            if before, after, ok := strings.Cut(line, "#!#"); ok {
                line = strings.TrimSpace(before)
                /* input id ignored in hybrid phases */
                _ = after
            }
            queries = append(queries, line)
        }
    }
    if err := scanner.Err(); err != nil {
        return err
    }
    if len(queries) == 0 {
        return fmt.Errorf("no queries provided for hybrid mode")
    }

    fmt.Fprintf(os.Stderr, "========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID] Extended Hybrid Mode Starting\n")
    fmt.Fprintf(os.Stderr, "[HYBRID] Queries: %d\n", len(queries))
    fmt.Fprintf(os.Stderr, "[HYBRID] Geo: %s, Zoom: %d, Radius: %.0fm\n", cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    fmt.Fprintf(os.Stderr, "========================================\n\n")

    // Phase 1a: Fast Mode (HTTP API)
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1a: Running Fast Mode (HTTP API)...\n")
    fastSeeds, err := runHybridFastPhase(ctx, queries, cfg.LangCode, cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    if err != nil {
        return fmt.Errorf("Phase 1a (Fast Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1a complete: collected %d seeds from Fast Mode\n\n", len(fastSeeds))

    // Phase 1b: Normal Mode (Browser)
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1b: Running Normal Mode (Browser)...\n")
    normalSeeds, err := runHybridNormalPhase(ctx, queries, cfg, writers)
    if err != nil {
        return fmt.Errorf("Phase 1b (Normal Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1b complete: collected %d seeds from Normal Mode\n\n", len(normalSeeds))

    // Phase 1c: Initial Nearby Mode (Browser)
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1c: Running Initial Nearby Mode (Browser)...\n")
    nearbySeeds, err := runHybridInitialNearbyPhase(ctx, queries, cfg, writers)
    if err != nil {
        return fmt.Errorf("Phase 1c (Initial Nearby Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1c complete: collected %d seeds from Initial Nearby Mode\n\n", len(nearbySeeds))

    // Combine all seeds from phases 1a, 1b, 1c
    allSeeds := make([]hybridSeed, 0, len(fastSeeds)+len(normalSeeds)+len(nearbySeeds))
    allSeeds = append(allSeeds, fastSeeds...)
    allSeeds = append(allSeeds, normalSeeds...)
    allSeeds = append(allSeeds, nearbySeeds...)

    fmt.Fprintf(os.Stderr, "========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 1 Summary:\n")
    fmt.Fprintf(os.Stderr, "[HYBRID]   Fast Mode:          %d seeds\n", len(fastSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID]   Normal Mode:        %d seeds\n", len(normalSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID]   Initial Nearby:     %d seeds\n", len(nearbySeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID]   Total Seeds:        %d\n", len(allSeeds))
    fmt.Fprintf(os.Stderr, "========================================\n\n")

    if len(allSeeds) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID] No seeds collected from any phase. Exiting.\n")
        return nil
    }

    // Phase 2: Nested Nearby Mode on all collected seeds
    fmt.Fprintf(os.Stderr, "[HYBRID] Phase 2: Creating Nested Nearby Mode jobs for all %d seeds...\n", len(allSeeds))
    dedup := deduper.New()
    exitMonitor := exiter.New()
    nestedNearbyJobs := buildHybridNearbyJobs(allSeeds, cfg, dedup, exitMonitor)
    fmt.Fprintf(os.Stderr, "[HYBRID] Created %d nested nearby jobs\n", len(nestedNearbyJobs))

    if len(nestedNearbyJobs) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID] No nested nearby jobs to execute. Finished.\n")
        return nil
    }
    exitMonitor.SetSeedCount(len(nestedNearbyJobs))

    opts := []func(*scrapemateapp.Config) error{ scrapemateapp.WithConcurrency(cfg.Concurrency) }
    
    // Apply exit-on-inactivity if configured, with minimum safety timeout based on depth
    if cfg.ExitOnInactivityDuration > 0 {
        minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute
        exitOnInactivity := cfg.ExitOnInactivityDuration
        if exitOnInactivity < minInactivityTimeout {
            fmt.Fprintf(os.Stderr, "[HYBRID] Warning: -exit-on-inactivity %v is too short for depth %d. Using minimum %v\n",
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
    if len(cfg.Proxies) > 0 { opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies)) }
    if !cfg.DisablePageReuse { opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithPageReuseLimit(200)) }

    matecfg, err := scrapemateapp.NewConfig(writers, opts...)
    if err != nil { return err }
    app, err := scrapemateapp.NewScrapeMateApp(matecfg)
    if err != nil { return err }
    defer app.Close()

    ctx2, cancel := context.WithCancel(ctx)
    defer cancel()
    exitMonitor.SetCancelFunc(cancel)
    go exitMonitor.Run(ctx2)

    fmt.Fprintf(os.Stderr, "[HYBRID] Executing Phase 2: %d nested nearby jobs...\n", len(nestedNearbyJobs))
    if err := app.Start(ctx2, nestedNearbyJobs...); err != nil {
        // Context cancellation is expected when exiter completes all jobs - not an error
        if !errors.Is(err, context.Canceled) {
            return err
        }
    }

    fmt.Fprintf(os.Stderr, "\n========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID] Extended Hybrid Mode Complete!\n")
    fmt.Fprintf(os.Stderr, "[HYBRID] Total seeds processed: %d\n", len(allSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID] Nested nearby jobs executed: %d\n", len(nestedNearbyJobs))
    fmt.Fprintf(os.Stderr, "========================================\n")

    Telemetry().Send(ctx, tlmt.NewEvent("extended_hybrid_runner", map[string]any{
        "fast_seeds":         len(fastSeeds),
        "normal_seeds":       len(normalSeeds),
        "nearby_seeds":       len(nearbySeeds),
        "total_seeds":        len(allSeeds),
        "nested_nearby_jobs": len(nestedNearbyJobs),
    }))
    return nil
}

// RunHybridWeb executes the extended hybrid workflow for the web runner.
// Mirrors RunHybridFile but accepts pre-parsed keyword slice and writers.
// Phases:
// 1a. Fast Mode (HTTP) on input parameters
// 1b. Normal Mode (Browser) on input parameters
// 1c. Initial Nearby Mode (Browser) on input parameters
// 2. Nested Nearby Mode (Browser) on all collected seed locations
func RunHybridWeb(ctx context.Context, cfg *Config, keywords []string, writers []scrapemate.ResultWriter) error {
    if len(keywords) == 0 {
        return fmt.Errorf("no keywords provided for hybrid mode")
    }

    fmt.Fprintf(os.Stderr, "========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Extended Hybrid Mode Starting\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Keywords: %d\n", len(keywords))
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Geo: %s, Zoom: %d, Radius: %.0fm\n", cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    fmt.Fprintf(os.Stderr, "========================================\n\n")

    // Phase 1a: Fast Mode (HTTP API)
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1a: Running Fast Mode (HTTP API)...\n")
    fastSeeds, err := runHybridFastPhase(ctx, keywords, cfg.LangCode, cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    if err != nil {
        return fmt.Errorf("Phase 1a (Fast Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1a complete: collected %d seeds from Fast Mode\n\n", len(fastSeeds))

    // Phase 1b: Normal Mode (Browser)
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1b: Running Normal Mode (Browser)...\n")
    normalSeeds, err := runHybridNormalPhase(ctx, keywords, cfg, writers)
    if err != nil {
        return fmt.Errorf("Phase 1b (Normal Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1b complete: collected %d seeds from Normal Mode\n\n", len(normalSeeds))

    // Phase 1c: Initial Nearby Mode (Browser)
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1c: Running Initial Nearby Mode (Browser)...\n")
    nearbySeeds, err := runHybridInitialNearbyPhase(ctx, keywords, cfg, writers)
    if err != nil {
        return fmt.Errorf("Phase 1c (Initial Nearby Mode) failed: %w", err)
    }
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1c complete: collected %d seeds from Initial Nearby Mode\n\n", len(nearbySeeds))

    // Combine all seeds from phases 1a, 1b, 1c
    allSeeds := make([]hybridSeed, 0, len(fastSeeds)+len(normalSeeds)+len(nearbySeeds))
    allSeeds = append(allSeeds, fastSeeds...)
    allSeeds = append(allSeeds, normalSeeds...)
    allSeeds = append(allSeeds, nearbySeeds...)

    fmt.Fprintf(os.Stderr, "========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 1 Summary:\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB]   Fast Mode:          %d seeds\n", len(fastSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB]   Normal Mode:        %d seeds\n", len(normalSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB]   Initial Nearby:     %d seeds\n", len(nearbySeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB]   Total Seeds:        %d\n", len(allSeeds))
    fmt.Fprintf(os.Stderr, "========================================\n\n")

    if len(allSeeds) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID-WEB] No seeds collected from any phase. Exiting.\n")
        return nil
    }

    // Phase 2: Nested Nearby Mode on all collected seeds
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Phase 2: Creating Nested Nearby Mode jobs for all %d seeds...\n", len(allSeeds))
    dedup := deduper.New()
    exitMonitor := exiter.New()
    nestedNearbyJobs := buildHybridNearbyJobs(allSeeds, cfg, dedup, exitMonitor)
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Created %d nested nearby jobs\n", len(nestedNearbyJobs))

    if len(nestedNearbyJobs) == 0 {
        fmt.Fprintf(os.Stderr, "[HYBRID-WEB] No nested nearby jobs to execute. Finished.\n")
        return nil
    }
    exitMonitor.SetSeedCount(len(nestedNearbyJobs))

    opts := []func(*scrapemateapp.Config) error{scrapemateapp.WithConcurrency(cfg.Concurrency)}

    // Apply exit-on-inactivity if configured, with minimum safety timeout based on depth
    if cfg.ExitOnInactivityDuration > 0 {
        minInactivityTimeout := time.Duration(cfg.MaxDepth*4)*time.Second + 2*time.Minute
        exitOnInactivity := cfg.ExitOnInactivityDuration
        if exitOnInactivity < minInactivityTimeout {
            fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Warning: -exit-on-inactivity %v is too short for depth %d. Using minimum %v\n",
                exitOnInactivity, cfg.MaxDepth, minInactivityTimeout)
            exitOnInactivity = minInactivityTimeout
        }
        opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
    }

    if cfg.Debug {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages()))
    } else {
        opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
    }
    if len(cfg.Proxies) > 0 {
        opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies))
    }
    if !cfg.DisablePageReuse {
        opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithPageReuseLimit(200))
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

    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Executing Phase 2: %d nested nearby jobs...\n", len(nestedNearbyJobs))
    if err := app.Start(ctx2, nestedNearbyJobs...); err != nil {
        // Context cancellation is expected when exiter completes all jobs - not an error
        if !errors.Is(err, context.Canceled) {
            return err
        }
    }

    fmt.Fprintf(os.Stderr, "\n========================================\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Extended Hybrid Mode Complete!\n")
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Total seeds processed: %d\n", len(allSeeds))
    fmt.Fprintf(os.Stderr, "[HYBRID-WEB] Nested nearby jobs executed: %d\n", len(nestedNearbyJobs))
    fmt.Fprintf(os.Stderr, "========================================\n")

    return nil
}
