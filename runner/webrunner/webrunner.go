package webrunner

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/deduper"
	"github.com/gosom/google-maps-scraper/exiter"
	"github.com/gosom/google-maps-scraper/postgres"
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/google-maps-scraper/web/sqlite"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/adapters/writers/csvwriter"
	"github.com/gosom/scrapemate/scrapemateapp"
	"golang.org/x/sync/errgroup"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

type webrunner struct {
	srv *web.Server
	svc *web.Service
	cfg *runner.Config
}

func New(cfg *runner.Config) (runner.Runner, error) {
	if cfg.DataFolder == "" {
		return nil, fmt.Errorf("data folder is required")
	}

	if err := os.MkdirAll(cfg.DataFolder, os.ModePerm); err != nil {
		return nil, err
	}

	// Check if PostgreSQL is configured (DATABASE_URL environment variable)
	var repo web.JobRepository
	var apiKeyRepo web.APIKeyRepository
	var authSvc *web.AuthService
	var svc *web.Service
	var apiKeySvc *web.APIKeyService

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		log.Printf("PostgreSQL configured (DATABASE_URL found)")
		pgDB, err := openPostgresConn(databaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}

		// Use PostgreSQL for everything
		repo = postgres.NewJobRepository(pgDB)
		apiKeyRepo = postgres.NewAPIKeyRepository(pgDB)
		svc = web.NewService(repo, cfg.DataFolder)
		apiKeySvc = web.NewAPIKeyService(apiKeyRepo)

		// Create PostgreSQL auth repositories
		userRepo := postgres.NewUserRepository(pgDB)
		sessionRepo := postgres.NewUserSessionRepository(pgDB)
		authSvc = web.NewAuthService(userRepo, sessionRepo, nil) // nil for audit repo
		log.Printf("PostgreSQL enabled for all data (jobs, API keys, authentication)")
	} else {
		// Fallback to SQLite
		log.Printf("No DATABASE_URL configured. Using SQLite.")
		const dbfname = "jobs.db"
		dbpath := filepath.Join(cfg.DataFolder, dbfname)

		db, err := sqlite.InitDB(dbpath)
		if err != nil {
			return nil, err
		}

		repo = sqlite.NewWithDB(db)
		svc = web.NewService(repo, cfg.DataFolder)
		apiKeyRepo = sqlite.NewAPIKeyRepository(db)
		apiKeySvc = web.NewAPIKeyService(apiKeyRepo)

		// Use SQLite for auth
		userRepo := sqlite.NewUserRepository(db)
		sessionRepo := sqlite.NewSessionRepository(db)
		authSvc = web.NewAuthService(userRepo, sessionRepo, nil)
	}

	// Create server with API key support and auth service
	srv, err := web.NewWithAPIKeysAndAuth(svc, apiKeySvc, authSvc, cfg.Addr)
	if err != nil {
		return nil, err
	}

	ans := webrunner{
		srv: srv,
		svc: svc,
		cfg: cfg,
	}

	return &ans, nil
}

// openPostgresConn opens a connection to PostgreSQL
func openPostgresConn(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	return db, nil
}

func (w *webrunner) Run(ctx context.Context) error {
	// Recover any jobs stuck in "working" status from previous server runs
	if err := w.recoverStuckJobsOnStartup(ctx); err != nil {
		log.Printf("WARNING: failed to recover stuck jobs on startup: %v", err)
	}

	egroup, ctx := errgroup.WithContext(ctx)

	egroup.Go(func() error {
		return w.work(ctx)
	})

	egroup.Go(func() error {
		return w.recoverStuckJobs(ctx)
	})

	egroup.Go(func() error {
		return w.srv.Start(ctx)
	})

	return egroup.Wait()
}

func (w *webrunner) Close(context.Context) error {
	return nil
}

func (w *webrunner) recoverStuckJobsOnStartup(ctx context.Context) error {
	// When the server starts, any jobs in "working" status must be from a previous
	// server run and should be marked as failed since they can't actually be running
	workingJobs, err := w.svc.SelectWorking(ctx)
	if err != nil {
		return fmt.Errorf("failed to select working jobs: %w", err)
	}

	if len(workingJobs) == 0 {
		log.Printf("startup recovery: no stuck jobs found")
		return nil
	}

	log.Printf("startup recovery: found %d stuck jobs, marking them as failed", len(workingJobs))

	for i := range workingJobs {
		job := &workingJobs[i]
		log.Printf("startup recovery: marking job %s (%s) as failed (was stuck in working status)", job.ID, job.Name)

		job.Status = web.StatusFailed
		if err := w.svc.Update(ctx, job); err != nil {
			log.Printf("startup recovery: failed to update job %s: %v", job.ID, err)
			continue
		}

		log.Printf("startup recovery: successfully recovered job %s", job.ID)
	}

	return nil
}

func (w *webrunner) work(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	log.Printf("job worker started, checking for pending jobs every second")

	for {
		select {
		case <-ctx.Done():
			log.Printf("job worker shutting down")
			return nil
		case <-ticker.C:
			jobs, err := w.svc.SelectPending(ctx)
			if err != nil {
				// Don't exit the work loop on error, just log it and continue
				log.Printf("error selecting pending jobs: %v", err)
				continue
			}

			if len(jobs) == 0 {
				// No pending jobs, continue waiting
				continue
			}

			for i := range jobs {
				select {
				case <-ctx.Done():
					return nil
				default:
					log.Printf("picked up job %s (%s) for processing", jobs[i].ID, jobs[i].Name)
					t0 := time.Now().UTC()
					if err := w.scrapeJob(ctx, &jobs[i]); err != nil {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
							"error":     err.Error(),
						}

						evt := tlmt.NewEvent("web_runner", params)

						_ = runner.Telemetry().Send(ctx, evt)

						log.Printf("ERROR: job %s (%s) failed after %v: %v", jobs[i].ID, jobs[i].Name, time.Now().UTC().Sub(t0), err)
					} else {
						params := map[string]any{
							"job_count": len(jobs[i].Data.Keywords),
							"duration":  time.Now().UTC().Sub(t0).String(),
						}

						_ = runner.Telemetry().Send(ctx, tlmt.NewEvent("web_runner", params))

						log.Printf("SUCCESS: job %s (%s) completed in %v", jobs[i].ID, jobs[i].Name, time.Now().UTC().Sub(t0))
					}
				}
			}
		}
	}
}

func (w *webrunner) recoverStuckJobs(ctx context.Context) error {
	// Check for stuck jobs every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	log.Printf("stuck job recovery mechanism started")

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			workingJobs, err := w.svc.SelectWorking(ctx)
			if err != nil {
				log.Printf("error selecting working jobs for recovery: %v", err)
				continue
			}

			now := time.Now().UTC()
			for i := range workingJobs {
				job := &workingJobs[i]

				// Calculate timeout: max of 1 hour or 2x the job's MaxTime
				timeout := time.Hour
				if job.Data.MaxTime > 0 {
					jobTimeout := 2 * job.Data.MaxTime
					if jobTimeout > timeout {
						timeout = jobTimeout
					}
				}

				// If job hasn't been updated in timeout period, mark as failed
				timeSinceUpdate := now.Sub(job.UpdatedAt)
				if timeSinceUpdate > timeout {
					log.Printf("recovering stuck job %s (stuck for %v, timeout: %v)", job.ID, timeSinceUpdate, timeout)

					job.Status = web.StatusFailed
					if err := w.svc.Update(ctx, job); err != nil {
						log.Printf("failed to recover stuck job %s: %v", job.ID, err)
					} else {
						log.Printf("successfully recovered stuck job %s", job.ID)
					}
				}
			}
		}
	}
}

func (w *webrunner) scrapeJob(ctx context.Context, job *web.Job) error {
	log.Printf("job %s: changing status to 'working'", job.ID)
	job.Status = web.StatusWorking

	// Use defer to ensure job status is always updated, even on panic or early returns
	var jobErr error
	defer func() {
		// If job status is still "working", it means something went wrong
		if job.Status == web.StatusWorking {
			job.Status = web.StatusFailed
			log.Printf("job %s: stuck in working status, marking as failed", job.ID)
		}

		// Always try to update the final status (best-effort)
		log.Printf("job %s: updating final status to '%s'", job.ID, job.Status)
		if err := w.svc.Update(ctx, job); err != nil {
			log.Printf("job %s: ERROR - failed to update final status: %v", job.ID, err)
		} else {
			log.Printf("job %s: status successfully updated to '%s'", job.ID, job.Status)
		}
	}()

	err := w.svc.Update(ctx, job)
	if err != nil {
		jobErr = err
		job.Status = web.StatusFailed
		log.Printf("job %s: ERROR - failed to update to 'working' status: %v", job.ID, err)
		return err
	}

	if len(job.Data.Keywords) == 0 {
		job.Status = web.StatusFailed
		jobErr = fmt.Errorf("no keywords provided")
		return jobErr
	}

	outpath := filepath.Join(w.cfg.DataFolder, job.ID+".csv")

	outfile, err := os.Create(outpath)
	if err != nil {
		job.Status = web.StatusFailed
		jobErr = err
		return err
	}

	defer func() {
		_ = outfile.Close()
	}()

	// Write UTF-8 BOM for proper encoding detection in Excel and other applications
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := outfile.Write(bom); err != nil {
		job.Status = web.StatusFailed
		jobErr = fmt.Errorf("failed to write UTF-8 BOM: %w", err)
		return jobErr
	}

	var mate *scrapemateapp.ScrapemateApp
	if !job.Data.HybridMode && !job.Data.BrowserAPIMode { // hybrid and browserapi build their own app internally
		mate, err = w.setupMate(ctx, outfile, job)
		if err != nil {
			job.Status = web.StatusFailed
			jobErr = err
			return err
		}
		defer mate.Close()
	}

	var coords string
	if job.Data.Lat != "" && job.Data.Lon != "" {
		coords = job.Data.Lat + "," + job.Data.Lon
	}

	dedup := deduper.New()
	exitMonitor := exiter.New()

	var seedJobs []scrapemate.IJob

	if job.Data.HybridMode {
		// Execute new hybrid workflow directly (fast API -> nearby browser)
		writers := []scrapemate.ResultWriter{csvwriter.NewCsvWriter(csv.NewWriter(outfile))}
		// Build a temporary config copy with job-specific parameters (lat/lon/zoom/radius/depth/email/lang)
		geo := coords // already constructed from job.Data.Lat/Lon above
		tmpCfg := *w.cfg
		tmpCfg.GeoCoordinates = geo
		tmpCfg.ZoomLevel = job.Data.Zoom
		tmpCfg.Radius = float64(job.Data.Radius)
		tmpCfg.Email = job.Data.Email
		tmpCfg.MaxDepth = job.Data.Depth
		tmpCfg.LangCode = job.Data.Lang
		tmpCfg.ExitOnInactivityDuration = job.Data.ExitOnInactivity
		// Override concurrency if job-specific value is set
		if job.Data.Concurrency > 0 {
			tmpCfg.Concurrency = job.Data.Concurrency
			log.Printf("job %s: using job-specific concurrency for hybrid mode: %d", job.ID, tmpCfg.Concurrency)
		} else {
			log.Printf("job %s: using global config concurrency for hybrid mode: %d", job.ID, tmpCfg.Concurrency)
		}
		// Override proxies if job-specific proxies are provided (API/Web UI takes priority)
		if len(job.Data.Proxies) > 0 {
			tmpCfg.Proxies = job.Data.Proxies
			log.Printf("job %s: using job-specific proxies for hybrid mode (%d proxies)", job.ID, len(job.Data.Proxies))
		} else if len(tmpCfg.Proxies) == 0 {
			// Fallback to .env PROXY if no CLI/config proxies
			envProxy := os.Getenv("PROXY")
			if envProxy != "" {
				tmpCfg.Proxies = []string{envProxy}
				log.Printf("job %s: using fallback proxy from .env for hybrid mode: %s", job.ID, envProxy)
			}
		}
		if err := runner.RunHybridWeb(ctx, &tmpCfg, job.Data.Keywords, writers); err != nil {
			job.Status = web.StatusFailed
			jobErr = err
			return err
		}
		job.Status = web.StatusOK
		return nil // Hybrid completed; normal seedJobs path skipped
	} else if job.Data.BrowserAPIMode {
		// Execute BrowserAPI workflow directly (Google Places API -> browser scrape -> nearby)
		writers := []scrapemate.ResultWriter{csvwriter.NewCsvWriter(csv.NewWriter(outfile))}
		// Build a temporary config copy with job-specific parameters
		geo := coords // already constructed from job.Data.Lat/Lon above
		tmpCfg := *w.cfg
		tmpCfg.GeoCoordinates = geo
		tmpCfg.ZoomLevel = job.Data.Zoom
		tmpCfg.Radius = float64(job.Data.Radius)
		tmpCfg.Email = job.Data.Email
		tmpCfg.MaxDepth = job.Data.Depth
		tmpCfg.LangCode = job.Data.Lang
		tmpCfg.ExitOnInactivityDuration = job.Data.ExitOnInactivity
		// Override concurrency if job-specific value is set
		if job.Data.Concurrency > 0 {
			tmpCfg.Concurrency = job.Data.Concurrency
			log.Printf("job %s: using job-specific concurrency for BrowserAPI mode: %d", job.ID, tmpCfg.Concurrency)
		} else {
			log.Printf("job %s: using global config concurrency for BrowserAPI mode: %d", job.ID, tmpCfg.Concurrency)
		}
		// Override proxies if job-specific proxies are provided (API/Web UI takes priority)
		if len(job.Data.Proxies) > 0 {
			tmpCfg.Proxies = job.Data.Proxies
			log.Printf("job %s: using job-specific proxies for BrowserAPI mode (%d proxies)", job.ID, len(job.Data.Proxies))
		} else if len(tmpCfg.Proxies) == 0 {
			// Fallback to .env PROXY if no CLI/config proxies
			envProxy := os.Getenv("PROXY")
			if envProxy != "" {
				tmpCfg.Proxies = []string{envProxy}
				log.Printf("job %s: using fallback proxy from .env for BrowserAPI mode: %s", job.ID, envProxy)
			}
		}
		if err := runner.RunBrowserAPIWeb(ctx, &tmpCfg, job.Data.Keywords, writers); err != nil {
			job.Status = web.StatusFailed
			jobErr = err
			return err
		}
		job.Status = web.StatusOK
		return nil // BrowserAPI completed; normal seedJobs path skipped
	} else if job.Data.NearbyMode {
		// Nearby mode: use CreateNearbySearchJobs
		seedJobs, err = runner.CreateNearbySearchJobs(
			job.Data.Lang,
			strings.NewReader(strings.Join(job.Data.Keywords, "\n")),
			job.Data.Depth,
			job.Data.Email,
			coords,
			func() float64 {
				if job.Data.Radius <= 0 {
					return 10000 // 10 km
				}
				return float64(job.Data.Radius)
			}(),
			float64(job.Data.Zoom), // In nearby mode, Zoom is interpreted as meters
			dedup,
			exitMonitor,
			w.cfg.ExtraReviews,
			w.cfg.GoogleMapsAPIKey,
		)
	} else {
		// Regular mode: use CreateSeedJobs
		seedJobs, err = runner.CreateSeedJobs(
			job.Data.FastMode,
			job.Data.Lang,
			strings.NewReader(strings.Join(job.Data.Keywords, "\n")),
			job.Data.Depth,
			job.Data.Email,
			coords,
			job.Data.Zoom, // In regular mode, Zoom is a zoom level (0-21)
			func() float64 {
				if job.Data.Radius <= 0 {
					return 10000 // 10 km
				}
				return float64(job.Data.Radius)
			}(),
			dedup,
			exitMonitor,
			w.cfg.ExtraReviews,
			w.cfg.GoogleMapsAPIKey,
		)
	}

	if err != nil {
		job.Status = web.StatusFailed
		jobErr = err
		return err
	}

	if len(seedJobs) > 0 {
		exitMonitor.SetSeedCount(len(seedJobs))

		allowedSeconds := max(60, len(seedJobs)*10*job.Data.Depth/50+120)

		if job.Data.MaxTime > 0 {
			if job.Data.MaxTime.Seconds() < 180 {
				allowedSeconds = 180
			} else {
				allowedSeconds = int(job.Data.MaxTime.Seconds())
			}
		}

		log.Printf("running job %s with %d seed jobs and %d allowed seconds", job.ID, len(seedJobs), allowedSeconds)

		mateCtx, cancel := context.WithTimeout(ctx, time.Duration(allowedSeconds)*time.Second)
		defer cancel()

		exitMonitor.SetCancelFunc(cancel)

		go exitMonitor.Run(mateCtx)

		// Run Start in a separate goroutine so we can detect hangs after context cancellation
		startDone := make(chan error, 1)
		go func() {
			startDone <- mate.Start(mateCtx, seedJobs...)
		}()

		var startErr error
		select {
		case startErr = <-startDone:
			// Start returned normally (success or error)
		case <-mateCtx.Done():
			// Context timed out/canceled; wait a short grace for Start to return, then proceed
			select {
			case startErr = <-startDone:
				// returned during grace
			case <-time.After(15 * time.Second):
				log.Printf("job %s: Start() did not return within grace period after context cancellation; proceeding", job.ID)
				startErr = mateCtx.Err()
			}
		}

		if startErr != nil && !errors.Is(startErr, context.DeadlineExceeded) && !errors.Is(startErr, context.Canceled) {
			cancel()
			job.Status = web.StatusFailed
			jobErr = startErr
			log.Printf("scrapemate failed for job %s: %v", job.ID, startErr)
			return startErr
		}

		cancel()
	}

	// At this point, scraping finished successfully. Mark job as OK
	job.Status = web.StatusOK

	// Persist status immediately before any potentially blocking cleanup
	if err := w.svc.Update(ctx, job); err != nil {
		log.Printf("job %s: WARNING - scrape finished but failed to persist 'ok' status: %v", job.ID, err)
	} else {
		log.Printf("job %s: persisted 'ok' status", job.ID)
	}

	// Best-effort close without risking a hang: attempt close with a short timeout
	closed := make(chan struct{})
	go func() {
		mate.Close()
		close(closed)
	}()
	select {
	case <-closed:
		// closed successfully
	case <-time.After(5 * time.Second):
		log.Printf("job %s: Close() taking too long; continuing shutdown", job.ID)
	}

	return nil
}

func (w *webrunner) setupMate(_ context.Context, writer io.Writer, job *web.Job) (*scrapemateapp.ScrapemateApp, error) {
	// Use job-specific concurrency if set, otherwise use global config
	concurrency := w.cfg.Concurrency
	if job.Data.Concurrency > 0 {
		concurrency = job.Data.Concurrency
		log.Printf("job %s: using job-specific concurrency: %d", job.ID, concurrency)
	} else {
		log.Printf("job %s: using global config concurrency: %d", job.ID, concurrency)
	}

	opts := []func(*scrapemateapp.Config) error{
		scrapemateapp.WithConcurrency(concurrency),
	}

	// Only use ExitOnInactivity if user explicitly sets it
	// The exiter package handles completion detection automatically
	// ExitOnInactivity is problematic during long BrowserActions (scrolling)
	// because scrapemate measures inactivity from first job completion,
	// but during scrolling no jobs complete yet
	if job.Data.ExitOnInactivity > 0 {
		exitOnInactivity := job.Data.ExitOnInactivity
		log.Printf("job %s: User provided ExitOnInactivity: %v", job.ID, exitOnInactivity)

		// For nearby mode with deep scrolling, calculate minimum safe timeout
		// Each scroll iteration can take 2-4 seconds, so we need at least (depth × 4s) + 2 minutes buffer
		if job.Data.NearbyMode && job.Data.Depth > 0 {
			minInactivityTimeout := time.Duration(job.Data.Depth*4)*time.Second + 2*time.Minute

			if exitOnInactivity < minInactivityTimeout {
				log.Printf("job %s: Warning: exit-on-inactivity %v is too short for depth %d. Using minimum %v",
					job.ID, exitOnInactivity, job.Data.Depth, minInactivityTimeout)
				exitOnInactivity = minInactivityTimeout
			}
		}

		log.Printf("job %s: Setting ExitOnInactivity to %v", job.ID, exitOnInactivity)
		opts = append(opts, scrapemateapp.WithExitOnInactivity(exitOnInactivity))
	} else {
		log.Printf("job %s: ExitOnInactivity not set by user, relying on exiter package for completion detection", job.ID)
	}

	if job.Data.HybridMode {
		// Hybrid mode: needs stealth for fast mode API calls AND JS for nearby browser automation
		opts = append(opts,
			scrapemateapp.WithStealth("firefox"),
			scrapemateapp.WithJS(scrapemateapp.DisableImages()),
		)
	} else if job.Data.FastMode {
		// Fast mode: only uses stealth (HTTP requests, no browser)
		opts = append(opts,
			scrapemateapp.WithStealth("firefox"),
		)
	} else {
		// Regular and nearby modes: use browser automation
		opts = append(opts,
			scrapemateapp.WithJS(scrapemateapp.DisableImages()),
		)
	}

	hasProxy := false

	if len(w.cfg.Proxies) > 0 {
		// Priority 1: CLI or global config proxies
		opts = append(opts, scrapemateapp.WithProxies(w.cfg.Proxies))
		hasProxy = true
		log.Printf("job %s: using CLI/config proxies (%d proxies)", job.ID, len(w.cfg.Proxies))
	} else if len(job.Data.Proxies) > 0 {
		// Priority 2: API/Web UI job-specific proxies
		opts = append(opts,
			scrapemateapp.WithProxies(job.Data.Proxies),
		)
		hasProxy = true
		log.Printf("job %s: using job-specific proxies (%d proxies)", job.ID, len(job.Data.Proxies))
	} else {
		// Priority 3: Fallback to .env PROXY variable
		envProxy := os.Getenv("PROXY")
		if envProxy != "" {
			opts = append(opts, scrapemateapp.WithProxies([]string{envProxy}))
			hasProxy = true
			log.Printf("job %s: using fallback proxy from .env: %s", job.ID, envProxy)
		}
	}

	if !w.cfg.DisablePageReuse {
		opts = append(opts,
			scrapemateapp.WithPageReuseLimit(2),
			scrapemateapp.WithPageReuseLimit(200),
		)
	}

	log.Printf("job %s has proxy: %v", job.ID, hasProxy)

	csvWriter := csvwriter.NewCsvWriter(csv.NewWriter(writer))

	writers := []scrapemate.ResultWriter{csvWriter}

	matecfg, err := scrapemateapp.NewConfig(
		writers,
		opts...,
	)
	if err != nil {
		return nil, err
	}

	return scrapemateapp.NewScrapeMateApp(matecfg)
}
