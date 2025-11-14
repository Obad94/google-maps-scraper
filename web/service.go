package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gosom/google-maps-scraper/gmaps"
)

type Service struct {
	repo       JobRepository
	dataFolder string
}

func NewService(repo JobRepository, dataFolder string) *Service {
	return &Service{
		repo:       repo,
		dataFolder: dataFolder,
	}
}

func (s *Service) Create(ctx context.Context, job *Job) error {
	return s.repo.Create(ctx, job)
}

func (s *Service) All(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{})
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.repo.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id string) error {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid file name")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")

	if _, err := os.Stat(datapath); err == nil {
		if err := os.Remove(datapath); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return s.repo.Delete(ctx, id)
}

func (s *Service) Update(ctx context.Context, job *Job) error {
	return s.repo.Update(ctx, job)
}

func (s *Service) SelectPending(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusPending, Limit: 1})
}

func (s *Service) SelectWorking(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusWorking})
}

func (s *Service) Retry(ctx context.Context, id string) error {
	// Get the job
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Only allow retrying failed jobs
	if job.Status != StatusFailed {
		return fmt.Errorf("only failed jobs can be retried, current status: %s", job.Status)
	}

	// Delete the old CSV file if it exists (partial data from failed run)
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid job id")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")
	if _, err := os.Stat(datapath); err == nil {
		if err := os.Remove(datapath); err != nil {
			return fmt.Errorf("failed to remove old csv file: %w", err)
		}
	}

	// Change status to pending so it will be picked up by the worker
	job.Status = StatusPending

	return s.repo.Update(ctx, &job)
}

func (s *Service) GetCSV(_ context.Context, id string) (string, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid file name")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")

	if _, err := os.Stat(datapath); os.IsNotExist(err) {
		return "", fmt.Errorf("csv file not found for job %s", id)
	}

	return datapath, nil
}

func (s *Service) GetResults(_ context.Context, id string) ([]gmaps.Entry, error) {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return nil, fmt.Errorf("invalid file name")
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")

	if _, err := os.Stat(datapath); os.IsNotExist(err) {
		return nil, fmt.Errorf("csv file not found for job %s", id)
	}

	file, err := os.Open(datapath)
	if err != nil {
		return nil, fmt.Errorf("failed to open csv file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	_, err = reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv header: %w", err)
	}

	var results []gmaps.Entry

	for {
		record, err := reader.Read()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, fmt.Errorf("failed to read csv record: %w", err)
		}

		if len(record) < 33 {
			continue
		}

		entry := gmaps.Entry{
			ID:          record[0],
			Link:        record[1],
			Title:       record[2],
			Category:    record[3],
			Address:     record[4],
			WebSite:     record[7],
			Phone:       record[8],
			PlusCode:    record[9],
			Cid:         record[15],
			Status:      record[16],
			Description: record[17],
			ReviewsLink: record[18],
			Thumbnail:   record[19],
			Timezone:    record[20],
			PriceRange:  record[21],
			DataID:      record[22],
		}

		// Parse numeric fields
		if record[10] != "" {
			entry.ReviewCount, _ = strconv.Atoi(record[10])
		}
		if record[11] != "" {
			entry.ReviewRating, _ = strconv.ParseFloat(record[11], 64)
		}
		if record[13] != "" {
			entry.Latitude, _ = strconv.ParseFloat(record[13], 64)
		}
		if record[14] != "" {
			entry.Longtitude, _ = strconv.ParseFloat(record[14], 64)
		}

		// Parse JSON fields
		if record[5] != "" && record[5] != "null" {
			_ = json.Unmarshal([]byte(record[5]), &entry.OpenHours)
		}
		if record[6] != "" && record[6] != "null" {
			_ = json.Unmarshal([]byte(record[6]), &entry.PopularTimes)
		}
		if record[12] != "" && record[12] != "null" {
			_ = json.Unmarshal([]byte(record[12]), &entry.ReviewsPerRating)
		}
		if record[23] != "" && record[23] != "null" {
			_ = json.Unmarshal([]byte(record[23]), &entry.Images)
		}
		if record[24] != "" && record[24] != "null" {
			_ = json.Unmarshal([]byte(record[24]), &entry.Reservations)
		}
		if record[25] != "" && record[25] != "null" {
			_ = json.Unmarshal([]byte(record[25]), &entry.OrderOnline)
		}
		if record[26] != "" && record[26] != "null" {
			_ = json.Unmarshal([]byte(record[26]), &entry.Menu)
		}
		if record[27] != "" && record[27] != "null" {
			_ = json.Unmarshal([]byte(record[27]), &entry.Owner)
		}
		if record[28] != "" && record[28] != "null" {
			_ = json.Unmarshal([]byte(record[28]), &entry.CompleteAddress)
		}
		if record[29] != "" && record[29] != "null" {
			_ = json.Unmarshal([]byte(record[29]), &entry.About)
		}
		if record[30] != "" && record[30] != "null" {
			_ = json.Unmarshal([]byte(record[30]), &entry.UserReviews)
		}
		if record[31] != "" && record[31] != "null" {
			_ = json.Unmarshal([]byte(record[31]), &entry.UserReviewsExtended)
		}
		if record[32] != "" && record[32] != "null" {
			entry.Emails = strings.Split(record[32], ", ")
		}

		results = append(results, entry)
	}

	return results, nil
}
