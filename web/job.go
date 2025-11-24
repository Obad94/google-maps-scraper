package web

import (
	"context"
	"errors"
	"time"
)

var jobs []Job

const (
	StatusPending = "pending"
	StatusWorking = "working"
	StatusOK      = "ok"
	StatusFailed  = "failed"
)

type SelectParams struct {
	Status string
	Limit  int
}

type JobRepository interface {
	Get(context.Context, string) (Job, error)
	Create(context.Context, *Job) error
	Delete(context.Context, string) error
	Select(context.Context, SelectParams) ([]Job, error)
	Update(context.Context, *Job) error
}

type Job struct {
	ID        string
	Name      string
	Date      time.Time
	UpdatedAt time.Time
	Status    string
	Data      JobData
}

func (j *Job) Validate() error {
	if j.ID == "" {
		return errors.New("missing id")
	}

	if j.Name == "" {
		return errors.New("missing name")
	}

	if j.Status == "" {
		return errors.New("missing status")
	}

	if j.Date.IsZero() {
		return errors.New("missing date")
	}

	if err := j.Data.Validate(); err != nil {
		return err
	}

	return nil
}

type JobData struct {
	Keywords           []string      `json:"keywords"`
	Lang               string        `json:"lang"`
	Zoom               int           `json:"zoom"`
	Lat                string        `json:"lat"`
	Lon                string        `json:"lon"`
	FastMode           bool          `json:"fast_mode"`
	NearbyMode         bool          `json:"nearby_mode"`
	HybridMode         bool          `json:"hybrid_mode"`
	Radius             int           `json:"radius"`
	Depth              int           `json:"depth"`
	Email              bool          `json:"email"`
	MaxTime            time.Duration `json:"max_time"`
	ExitOnInactivity   time.Duration `json:"exit_on_inactivity"`
	Proxies            []string      `json:"proxies"`
	Concurrency        int           `json:"concurrency"`
}

func (d *JobData) Validate() error {
	if len(d.Keywords) == 0 {
		return errors.New("missing keywords")
	}

	if d.Lang == "" {
		return errors.New("missing lang")
	}

	if len(d.Lang) != 2 {
		return errors.New("invalid lang")
	}

	// Count active modes
	modeCount := 0
	if d.FastMode {
		modeCount++
	}
	if d.NearbyMode {
		modeCount++
	}
	if d.HybridMode {
		modeCount++
	}

	// Validate mode exclusivity
	if modeCount > 1 {
		return errors.New("cannot enable multiple modes (fast, nearby, hybrid) at the same time")
	}

	// Validate zoom based on mode
	if d.NearbyMode {
		// In nearby mode, zoom is interpreted as meters (must be 51+)
		if d.Zoom < 51 {
			return errors.New("zoom must be 51 or greater (meters) in nearby mode")
		}
	} else {
		// In regular, fast, and hybrid modes, zoom is a level (0-21)
		if d.Zoom < 0 || d.Zoom > 21 {
			return errors.New("zoom must be between 0 and 21 in regular/fast/hybrid mode")
		}
	}

	if d.Depth == 0 {
		return errors.New("missing depth")
	}

	if d.MaxTime == 0 {
		return errors.New("missing max time")
	}

	// Validate mode-specific requirements
	if d.FastMode && (d.Lat == "" || d.Lon == "") {
		return errors.New("missing geo coordinates for fast mode")
	}

	if d.NearbyMode && (d.Lat == "" || d.Lon == "") {
		return errors.New("missing geo coordinates for nearby mode")
	}

	if d.HybridMode && (d.Lat == "" || d.Lon == "") {
		return errors.New("missing geo coordinates for hybrid mode")
	}

	return nil
}
