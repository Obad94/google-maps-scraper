package runner

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"plugin"
	"strconv"
	"strings"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/gmaps"
	"github.com/gosom/scrapemate"
)

func CreateHybridSearchJobs(
	langCode string,
	r io.Reader,
	maxDepth int,
	email bool,
	geoCoordinates string,
	zoom int,
	radius float64,
	dedup deduper.Deduper,
	exitMonitor exiter.Exiter,
	extraReviews bool,
	googleMapsAPIKey string,
) (jobs []scrapemate.IJob, err error) {
	fmt.Fprintf(os.Stderr, "[HYBRID] Creating hybrid search jobs...\n")
	fmt.Fprintf(os.Stderr, "[HYBRID] Parameters: geo=%s, zoom=%dz, radius=%.0fm, depth=%d, email=%v, lang=%s\n",
		geoCoordinates, zoom, radius, maxDepth, email, langCode)

	if geoCoordinates == "" {
		return nil, fmt.Errorf("geo coordinates are required for hybrid mode")
	}

	parts := strings.Split(geoCoordinates, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid geo coordinates: %s", geoCoordinates)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %w", err)
	}

	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("invalid latitude: %f", lat)
	}

	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("invalid longitude: %f", lon)
	}

	if zoom < 1 || zoom > 21 {
		return nil, fmt.Errorf("invalid zoom level for hybrid mode: %d (must be 1-21)", zoom)
	}

	fmt.Fprintf(os.Stderr, "[HYBRID] Validated coordinates: lat=%.6f, lon=%.6f\n", lat, lon)

	// Calculate approximate meters for nearby phase (for logging)
	zoomMeters := ConvertZoomToMeters(zoom, lat)
	fmt.Fprintf(os.Stderr, "[HYBRID] Zoom conversion: %dz -> %dm for nearby search phase\n", zoom, zoomMeters)

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}

		var id string

		if before, after, ok := strings.Cut(query, "#!#"); ok {
			query = strings.TrimSpace(before)
			id = strings.TrimSpace(after)
		}

		opts := []gmaps.HybridJobOptions{}

		if dedup != nil {
			opts = append(opts, gmaps.WithHybridDeduper(dedup))
		}

		if exitMonitor != nil {
			opts = append(opts, gmaps.WithHybridExitMonitor(exitMonitor))
		}

		if extraReviews {
			opts = append(opts, gmaps.WithHybridExtraReviews())
		}

		if googleMapsAPIKey != "" {
			opts = append(opts, gmaps.WithHybridGoogleMapsAPIKey(googleMapsAPIKey))
		}

		job := gmaps.NewHybridJob(
			id,
			langCode,
			query,
			lat,
			lon,
			zoom,
			radius,
			maxDepth,
			email,
			opts...,
		)
		jobs = append(jobs, job)
		fmt.Fprintf(os.Stderr, "[HYBRID] Created hybrid job for query: '%s'\n", query)
	}

	fmt.Fprintf(os.Stderr, "[HYBRID] Total hybrid jobs created: %d\n", len(jobs))
	fmt.Fprintf(os.Stderr, "[HYBRID] Workflow: Fast mode (0-21 results) -> Nearby search at each location -> Full place details\n")

	return jobs, scanner.Err()
}

func CreateNearbySearchJobs(
	langCode string,
	r io.Reader,
	maxDepth int,
	email bool,
	geoCoordinates string,
	radius float64,
	zoomForURL float64,
	dedup deduper.Deduper,
	exitMonitor exiter.Exiter,
	extraReviews bool,
	googleMapsAPIKey string,
) (jobs []scrapemate.IJob, err error) {
	if geoCoordinates == "" {
		return nil, fmt.Errorf("geo coordinates are required for nearby search mode")
	}

	parts := strings.Split(geoCoordinates, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid geo coordinates: %s", geoCoordinates)
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return nil, fmt.Errorf("invalid longitude: %w", err)
	}

	if lat < -90 || lat > 90 {
		return nil, fmt.Errorf("invalid latitude: %f", lat)
	}

	if lon < -180 || lon > 180 {
		return nil, fmt.Errorf("invalid longitude: %f", lon)
	}

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		category := strings.TrimSpace(scanner.Text())
		if category == "" {
			continue
		}

		var id string

		if before, after, ok := strings.Cut(category, "#!#"); ok {
			category = strings.TrimSpace(before)
			id = strings.TrimSpace(after)
		}

		opts := []gmaps.NearbySearchJobOptions{}

		if dedup != nil {
			opts = append(opts, gmaps.WithNearbyDeduper(dedup))
		}

		if exitMonitor != nil {
			opts = append(opts, gmaps.WithNearbyExitMonitor(exitMonitor))
		}

		if extraReviews {
			opts = append(opts, gmaps.WithNearbyExtraReviews())
		}

		if radius > 0 {
			opts = append(opts, gmaps.WithNearbyRadiusFiltering(lat, lon, radius))
		}

		if zoomForURL > 0 {
			opts = append(opts, gmaps.WithNearbyZoom(zoomForURL))
		}

		if googleMapsAPIKey != "" {
			opts = append(opts, gmaps.WithNearbyGoogleMapsAPIKey(googleMapsAPIKey))
		}

		job := gmaps.NewNearbySearchJob(id, langCode, lat, lon, category, maxDepth, email, opts...)
		jobs = append(jobs, job)
	}

	return jobs, scanner.Err()
}

func CreateSeedJobs(
	fastmode bool,
	langCode string,
	r io.Reader,
	maxDepth int,
	email bool,
	geoCoordinates string,
	zoom int,
	radius float64,
	dedup deduper.Deduper,
	exitMonitor exiter.Exiter,
	extraReviews bool,
	googleMapsAPIKey string,
) (jobs []scrapemate.IJob, err error) {
	var lat, lon float64

	if fastmode {
		if geoCoordinates == "" {
			return nil, fmt.Errorf("geo coordinates are required in fast mode")
		}

		parts := strings.Split(geoCoordinates, ",")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid geo coordinates: %s", geoCoordinates)
		}

		lat, err = strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid latitude: %w", err)
		}

		lon, err = strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid longitude: %w", err)
		}

		if lat < -90 || lat > 90 {
			return nil, fmt.Errorf("invalid latitude: %f", lat)
		}

		if lon < -180 || lon > 180 {
			return nil, fmt.Errorf("invalid longitude: %f", lon)
		}

		if zoom < 1 || zoom > 21 {
			return nil, fmt.Errorf("invalid zoom level: %d", zoom)
		}

		if radius < 0 {
			return nil, fmt.Errorf("invalid radius: %f", radius)
		}
	}

	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		query := strings.TrimSpace(scanner.Text())
		if query == "" {
			continue
		}

		var id string

		if before, after, ok := strings.Cut(query, "#!#"); ok {
			query = strings.TrimSpace(before)
			id = strings.TrimSpace(after)
		}

		var job scrapemate.IJob

		if !fastmode {
			// In normal mode, if geo coords and a positive radius are provided,
			// generate multiple seed jobs that tile the desired radius, so we
			// effectively pan the map and collect results outside a single viewport.
			opts := []gmaps.GmapJobOptions{}

			if dedup != nil {
				opts = append(opts, gmaps.WithDeduper(dedup))
			}

			if exitMonitor != nil {
				opts = append(opts, gmaps.WithExitMonitor(exitMonitor))
			}

			if extraReviews {
				opts = append(opts, gmaps.WithExtraReviews())
			}

			if googleMapsAPIKey != "" {
				opts = append(opts, gmaps.WithGmapGoogleMapsAPIKey(googleMapsAPIKey))
			}

			// Add radius filtering if coordinates and radius are provided
			origLat, origLon := 0.0, 0.0
			latOK, lonOK := false, false
			if geoCoordinates != "" && radius > 0 {
				parts := strings.Split(geoCoordinates, ",")
				if len(parts) == 2 {
					if latParsed, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64); err1 == nil {
						origLat = latParsed
						latOK = true
					}
					if lonParsed, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err2 == nil {
						origLon = lonParsed
						lonOK = true
					}
				}
			}

			if geoCoordinates != "" && radius > 0 && latOK && lonOK {
				// Ensure place-level filtering enforces the true radius from the original center
				opts = append(opts, gmaps.WithRadiusFiltering(origLat, origLon, radius))

				// Generate tile centers to cover the circle; reuse the same input id to group outputs
				centers := generateTileCenters(origLat, origLon, radius)
				for _, c := range centers {
					gc := fmt.Sprintf("%f,%f", c[0], c[1])
					jobs = append(jobs, gmaps.NewGmapJob(id, langCode, query, maxDepth, email, gc, zoom, opts...))
				}

				// Continue to next keyword; we already appended jobs for all tiles
				continue
			}

			// Fallback: no radius tiling, create a single job centered on provided geo (or none)
			job = gmaps.NewGmapJob(id, langCode, query, maxDepth, email, geoCoordinates, zoom, opts...)
		} else {
			jparams := gmaps.MapSearchParams{
				Location: gmaps.MapLocation{
					Lat:     lat,
					Lon:     lon,
					ZoomLvl: float64(zoom),
					Radius:  radius,
				},
				Query:     query,
				ViewportW: 1920,
				ViewportH: 450,
				Hl:        langCode,
			}

			opts := []gmaps.SearchJobOptions{}

			if exitMonitor != nil {
				opts = append(opts, gmaps.WithSearchJobExitMonitor(exitMonitor))
			}

			job = gmaps.NewSearchJob(&jparams, opts...)
		}

		jobs = append(jobs, job)
	}

	return jobs, scanner.Err()
}

// generateTileCenters returns a list of [lat, lon] points that cover a circle of
// radiusMeters around (lat0, lon0). We use a simple overlapping grid so the
// normal-mode crawler can load multiple map views and escape a single viewport.
// The step is roughly radius/3 with a floor to avoid excessive tiles for small
// radii. The result count is capped implicitly by the grid size.
func generateTileCenters(lat0, lon0, radiusMeters float64) [][2]float64 {
	if radiusMeters <= 0 {
		return [][2]float64{{lat0, lon0}}
	}

	// meters to degrees conversions
	const metersPerDegLat = 111320.0
	metersPerDegLon := 111320.0 * math.Cos(lat0*math.Pi/180.0)
	if metersPerDegLon < 1e-6 {
		metersPerDegLon = 1e-6
	}

	// Choose an overlapping step; about 1/3 of the radius provides ~7x7 max grid
	step := radiusMeters / 3.0
	if step < 800 { // avoid too many tiles on very small radius
		step = 800
	}

	centers := make([][2]float64, 0, 49)
	// Always include the center
	centers = append(centers, [2]float64{lat0, lon0})

	r2 := radiusMeters * radiusMeters
	for dy := -radiusMeters; dy <= radiusMeters; dy += step {
		for dx := -radiusMeters; dx <= radiusMeters; dx += step {
			if dx == 0 && dy == 0 {
				continue
			}
			if dx*dx+dy*dy > r2 {
				continue
			}
			dLat := dy / metersPerDegLat
			dLon := dx / metersPerDegLon
			centers = append(centers, [2]float64{lat0 + dLat, lon0 + dLon})
		}
	}

	return centers
}

func LoadCustomWriter(pluginDir, pluginName string) (scrapemate.ResultWriter, error) {
	files, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) != ".so" && filepath.Ext(file.Name()) != ".dll" {
			continue
		}

		pluginPath := filepath.Join(pluginDir, file.Name())

		p, err := plugin.Open(pluginPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open plugin %s: %w", file.Name(), err)
		}

		symWriter, err := p.Lookup(pluginName)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup symbol %s: %w", pluginName, err)
		}

		writer, ok := symWriter.(*scrapemate.ResultWriter)
		if !ok {
			return nil, fmt.Errorf("unexpected type %T from writer symbol in plugin %s", symWriter, file.Name())
		}

		return *writer, nil
	}

	return nil, fmt.Errorf("no plugin found in %s", pluginDir)
}
