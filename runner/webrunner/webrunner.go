package webrunner

import (
	"context"
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
	"github.com/gosom/google-maps-scraper/runner"
	"github.com/gosom/google-maps-scraper/tlmt"
	"github.com/gosom/google-maps-scraper/web"
	"github.com/gosom/google-maps-scraper/web/sqlite"
	"github.com/gosom/scrapemate"
	"github.com/gosom/scrapemate/adapters/writers/csvwriter"
	"github.com/gosom/scrapemate/scrapemateapp"
	"golang.org/x/sync/errgroup"
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

	const dbfname = "jobs.db"

	dbpath := filepath.Join(cfg.DataFolder, dbfname)

	repo, err := sqlite.New(dbpath)
	if err != nil {
		return nil, err
	}

	svc := web.NewService(repo, cfg.DataFolder)

	srv, err := web.New(svc, cfg.Addr)
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

func (w *webrunner) Run(ctx context.Context) error {
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

		// Always try to update the final status
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

	mate, err := w.setupMate(ctx, outfile, job)
	if err != nil {
		job.Status = web.StatusFailed
		jobErr = err
		return err
	}

	defer mate.Close()

	var coords string
	if job.Data.Lat != "" && job.Data.Lon != "" {
		coords = job.Data.Lat + "," + job.Data.Lon
	}

	dedup := deduper.New()
	exitMonitor := exiter.New()

	seedJobs, err := runner.CreateSeedJobs(
		job.Data.FastMode,
		job.Data.Lang,
		strings.NewReader(strings.Join(job.Data.Keywords, "\n")),
		job.Data.Depth,
		job.Data.Email,
		coords,
		job.Data.Zoom,
		func() float64 {
			if job.Data.Radius <= 0 {
				return 10000 // 10 km
			}

			return float64(job.Data.Radius)
		}(),
		dedup,
		exitMonitor,
		w.cfg.ExtraReviews,
	)
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

		err = mate.Start(mateCtx, seedJobs...)
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			cancel()
			job.Status = web.StatusFailed
			jobErr = err
			log.Printf("scrapemate failed for job %s: %v", job.ID, err)
			return err
		}

		cancel()
	}

	mate.Close()

	// Success - mark as completed
	job.Status = web.StatusOK

	return nil
}

func (w *webrunner) setupMate(_ context.Context, writer io.Writer, job *web.Job) (*scrapemateapp.ScrapemateApp, error) {
	opts := []func(*scrapemateapp.Config) error{
		scrapemateapp.WithConcurrency(w.cfg.Concurrency),
		scrapemateapp.WithExitOnInactivity(time.Minute * 3),
	}

	if !job.Data.FastMode {
		opts = append(opts,
			scrapemateapp.WithJS(scrapemateapp.DisableImages()),
		)
	} else {
		opts = append(opts,
			scrapemateapp.WithStealth("firefox"),
		)
	}

	hasProxy := false

	if len(w.cfg.Proxies) > 0 {
		opts = append(opts, scrapemateapp.WithProxies(w.cfg.Proxies))
		hasProxy = true
	} else if len(job.Data.Proxies) > 0 {
		opts = append(opts,
			scrapemateapp.WithProxies(job.Data.Proxies),
		)
		hasProxy = true
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
