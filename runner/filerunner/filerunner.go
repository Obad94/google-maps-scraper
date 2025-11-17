package filerunner

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/adapters/writers/csvwriter"
	"github.com/gosom/scrapemate/adapters/writers/jsonwriter"
	"github.com/gosom/scrapemate/scrapemateapp"
)

type fileRunner struct {
	cfg     *runner.Config
	input   io.Reader
	writers []scrapemate.ResultWriter
	app     *scrapemateapp.ScrapemateApp
	outfile *os.File
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.RunMode != runner.RunModeFile {
		return nil, fmt.Errorf("%w: %d", runner.ErrInvalidRunMode, cfg.RunMode)
	}

	ans := &fileRunner{
		cfg: cfg,
	}

	if err := ans.setInput(); err != nil {
		return nil, err
	}

	if err := ans.setWriters(); err != nil {
		return nil, err
	}

	if err := ans.setApp(); err != nil {
		return nil, err
	}

	return ans, nil
}

func (r *fileRunner) Run(ctx context.Context) (err error) {
	var seedJobs []scrapemate.IJob

	t0 := time.Now().UTC()

	defer func() {
		elapsed := time.Now().UTC().Sub(t0)
		params := map[string]any{
			"job_count": len(seedJobs),
			"duration":  elapsed.String(),
		}

		if err != nil {
			params["error"] = err.Error()
		}

		evt := tlmt.NewEvent("file_runner", params)

		_ = runner.Telemetry().Send(ctx, evt)
	}()

	dedup := deduper.New()
	exitMonitor := exiter.New()

	if r.cfg.NearbyMode {
		seedJobs, err = runner.CreateNearbySearchJobs(
			r.cfg.LangCode,
			r.input,
			r.cfg.MaxDepth,
			r.cfg.Email,
			r.cfg.GeoCoordinates,
			r.cfg.Radius,
			dedup,
			exitMonitor,
			r.cfg.ExtraReviews,
		)
		if err != nil {
			return err
		}
	} else {
		seedJobs, err = runner.CreateSeedJobs(
			r.cfg.FastMode,
			r.cfg.LangCode,
			r.input,
			r.cfg.MaxDepth,
			r.cfg.Email,
			r.cfg.GeoCoordinates,
			r.cfg.Zoom,
			r.cfg.Radius,
			dedup,
			exitMonitor,
			r.cfg.ExtraReviews,
		)
		if err != nil {
			return err
		}
	}

	exitMonitor.SetSeedCount(len(seedJobs))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	exitMonitor.SetCancelFunc(cancel)

	go exitMonitor.Run(ctx)

	err = r.app.Start(ctx, seedJobs...)

	return err
}

func (r *fileRunner) Close(context.Context) error {
	if r.app != nil {
		return r.app.Close()
	}

	if r.input != nil {
		if closer, ok := r.input.(io.Closer); ok {
			return closer.Close()
		}
	}

	if r.outfile != nil {
		return r.outfile.Close()
	}

	return nil
}

func (r *fileRunner) setInput() error {
	switch r.cfg.InputFile {
	case "stdin":
		r.input = os.Stdin
	default:
		f, err := os.Open(r.cfg.InputFile)
		if err != nil {
			return err
		}

		r.input = f
	}

	return nil
}

func (r *fileRunner) setWriters() error {
	if r.cfg.CustomWriter != "" {
		parts := strings.Split(r.cfg.CustomWriter, ":")
		if len(parts) != 2 {
			return fmt.Errorf("invalid custom writer format: %s", r.cfg.CustomWriter)
		}

		dir, pluginName := parts[0], parts[1]

		customWriter, err := runner.LoadCustomWriter(dir, pluginName)
		if err != nil {
			return err
		}

		r.writers = append(r.writers, customWriter)
	} else {
		var resultsWriter io.Writer

		switch r.cfg.ResultsFile {
		case "stdout":
			resultsWriter = os.Stdout
		default:
			f, err := os.Create(r.cfg.ResultsFile)
			if err != nil {
				return err
			}

			r.outfile = f

			resultsWriter = r.outfile
		}

		// Write UTF-8 BOM for proper encoding detection in Excel and other applications
		if !r.cfg.JSON && resultsWriter != os.Stdout {
			bom := []byte{0xEF, 0xBB, 0xBF}
			if _, err := resultsWriter.Write(bom); err != nil {
				return fmt.Errorf("failed to write UTF-8 BOM: %w", err)
			}
		}

		csvWriter := csvwriter.NewCsvWriter(csv.NewWriter(resultsWriter))

		if r.cfg.JSON {
			r.writers = append(r.writers, jsonwriter.NewJSONWriter(resultsWriter))
		} else {
			r.writers = append(r.writers, csvWriter)
		}
	}

	return nil
}

func (r *fileRunner) setApp() error {
	opts := []func(*scrapemateapp.Config) error{
		// scrapemateapp.WithCache("leveldb", "cache"),
		scrapemateapp.WithConcurrency(r.cfg.Concurrency),
	}

	// Only use ExitOnInactivity if user explicitly sets it
	// The exiter package handles completion detection automatically
	// ExitOnInactivity is problematic during long BrowserActions (scrolling)
	// because scrapemate measures inactivity from first job completion,
	// but during scrolling no jobs complete yet
	if r.cfg.ExitOnInactivityDuration > 0 {
		// Calculate minimum safe timeout based on depth
		// Each scroll iteration can take 2-4 seconds, so we need at least (depth × 4s) + 2 minutes buffer
		minInactivityTimeout := time.Duration(r.cfg.MaxDepth*4)*time.Second + 2*time.Minute
		
		exitOnInactivity := r.cfg.ExitOnInactivityDuration
		if exitOnInactivity < minInactivityTimeout {
			fmt.Fprintf(os.Stderr, "Warning: -exit-on-inactivity %v is too short for depth %d. Using minimum %v\n",
				exitOnInactivity, r.cfg.MaxDepth, minInactivityTimeout)
			exitOnInactivity = minInactivityTimeout
		}
		
		opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
	}

	if len(r.cfg.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(r.cfg.Proxies),
		)
	}

	if !r.cfg.FastMode {
		if r.cfg.Debug {
			opts = append(opts, scrapemateapp.WithJS(
				scrapemateapp.Headfull(),
				scrapemateapp.DisableImages(),
			),
			)
		} else {
			opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
		}
	} else {
		// Fast mode uses HTTP requests, not browser automation
		opts = append(opts, scrapemateapp.WithStealth("firefox"))
	}

	// Nearby mode always requires browser automation for UI interaction
	if r.cfg.NearbyMode {
		if r.cfg.Debug {
			opts = append(opts, scrapemateapp.WithJS(
				scrapemateapp.Headfull(),
				scrapemateapp.DisableImages(),
			),
			)
		} else {
			opts = append(opts, scrapemateapp.WithJS(scrapemateapp.DisableImages()))
		}
	}

	if !r.cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithPageReuseLimit(200),
		)
	}

	matecfg, err := scrapemateapp.NewConfig(
		r.writers,
		opts...,
	)
	if err != nil {
		return err
	}

	r.app, err = scrapemateapp.NewScrapeMateApp(matecfg)
	if err != nil {
		return err
	}

	return nil
}
