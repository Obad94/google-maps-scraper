package runner

// Hybrid workflow implementation that keeps the original Fast Mode API (HTTP)
// for the first phase and then launches a second, fully browser/JS powered
// Nearby Search phase for every coordinate returned by Fast Mode.
//
// Rationale:
// - Scrapemate cannot mix stealth/HTTP and browser/JS modes reliably in a
//   single run (reported breakage of Fast Mode when both enabled).
// - We therefore execute Fast Mode outside scrapemate using the existing
//   SearchJob parsing logic, then build NearbySearchJobs and execute them in
//   a separate scrapemate session with JS enabled.
// - Only the detailed place results (from the Nearby + Place jobs) are written
//   to user writers; the initial fast mode seed list is used purely as input.
//
// This file adds two helper functions used by both the file runner and the
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

// runHybridFastPhase executes the Fast Mode API (HTTP) for each query and
// returns the collected seed entries tagged with their originating query.
// Fast mode returns up to ~21 results per query ordered by distance.
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

// RunHybridFile executes the hybrid workflow for the file runner.
func RunHybridFile(ctx context.Context, cfg *Config, input io.Reader, writers []scrapemate.ResultWriter) error {
    // Gather queries from input reader
    scanner := bufio.NewScanner(input)
    queries := []string{}
    for scanner.Scan() { line := strings.TrimSpace(scanner.Text()); if line != "" { if before, after, ok := strings.Cut(line, "#!#"); ok { line = strings.TrimSpace(before); /* input id ignored in hybrid fast phase */ _ = after } ; queries = append(queries, line) } }
    if err := scanner.Err(); err != nil { return err }
    if len(queries) == 0 { return fmt.Errorf("no queries provided for hybrid mode") }

    fmt.Fprintf(os.Stderr, "[HYBRID] Starting fast phase for %d queries...\n", len(queries))
    seeds, err := runHybridFastPhase(ctx, queries, cfg.LangCode, cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    if err != nil { return err }
    fmt.Fprintf(os.Stderr, "[HYBRID] Fast phase collected %d seed locations\n", len(seeds))

    dedup := deduper.New()
    exitMonitor := exiter.New()
    nearbyJobs := buildHybridNearbyJobs(seeds, cfg, dedup, exitMonitor)
    fmt.Fprintf(os.Stderr, "[HYBRID] Created %d nearby jobs from fast mode seeds\n", len(nearbyJobs))

    if len(nearbyJobs) == 0 { fmt.Fprintf(os.Stderr, "[HYBRID] No nearby jobs to execute (no seeds). Finished.\n"); return nil }
    exitMonitor.SetSeedCount(len(nearbyJobs))

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

    if err := app.Start(ctx2, nearbyJobs...); err != nil {
        // Context cancellation is expected when exiter completes all jobs - not an error
        if !errors.Is(err, context.Canceled) {
            return err
        }
    }
    Telemetry().Send(ctx, tlmt.NewEvent("hybrid_runner", map[string]any{"seeds": len(seeds), "nearby_jobs": len(nearbyJobs)}))
    return nil
}

// RunHybridWeb mirrors RunHybridFile but accepts pre-parsed keyword slice and writers.
func RunHybridWeb(ctx context.Context, cfg *Config, keywords []string, writers []scrapemate.ResultWriter) error {
    seeds, err := runHybridFastPhase(ctx, keywords, cfg.LangCode, cfg.GeoCoordinates, cfg.ZoomLevel, cfg.Radius)
    if err != nil { return err }
    fmt.Fprintf(os.Stderr, "[HYBRID] (web) Fast phase collected %d seed locations\n", len(seeds))
    dedup := deduper.New()
    exitMonitor := exiter.New()
    nearbyJobs := buildHybridNearbyJobs(seeds, cfg, dedup, exitMonitor)
    if len(nearbyJobs) == 0 { fmt.Fprintf(os.Stderr, "[HYBRID] (web) No nearby jobs generated\n"); return nil }
    exitMonitor.SetSeedCount(len(nearbyJobs))

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
    
    if cfg.Debug { opts = append(opts, scrapemateapp.WithJS(scrapemateapp.Headfull(), scrapemateapp.DisableImages())) } else { opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages())) }
    if len(cfg.Proxies) > 0 { opts = append(opts, scrapemateapp.WithProxies(cfg.Proxies)) }
    if !cfg.DisablePageReuse { opts = append(opts, scrapemateapp.WithPageReuseLimit(2), scrapemateapp.WithPageReuseLimit(200)) }
    matecfg, err := scrapemateapp.NewConfig(writers, opts...)
    if err != nil { return err }
    app, err := scrapemateapp.NewScrapeMateApp(matecfg)
    if err != nil { return err }
    defer app.Close()
    ctx2, cancel := context.WithCancel(ctx); defer cancel(); exitMonitor.SetCancelFunc(cancel); go exitMonitor.Run(ctx2)
    if err := app.Start(ctx2, nearbyJobs...); err != nil {
        // Context cancellation is expected when exiter completes all jobs - not an error
        if !errors.Is(err, context.Canceled) {
            return err
        }
    }
    return nil
}
