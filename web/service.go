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

		if len(record) < 35 {
			continue
		}

		entry := gmaps.Entry{
			ID:          record[0],
			Link:        record[1],
			PlaceID:     record[2],
			PlaceIDURL:  record[3],
			Title:       record[4],
			Category:    record[5],
			Address:     record[6],
			WebSite:     record[9],
			Phone:       record[10],
			PlusCode:    record[11],
			Cid:         record[17],
			Status:      record[18],
			Description: record[19],
			ReviewsLink: record[20],
			Thumbnail:   record[21],
			Timezone:    record[22],
			PriceRange:  record[23],
			DataID:      record[24],
		}

		// Parse numeric fields
		if record[12] != "" {
			entry.ReviewCount, _ = strconv.Atoi(record[12])
		}
		if record[13] != "" {
			entry.ReviewRating, _ = strconv.ParseFloat(record[13], 64)
		}
		if record[15] != "" {
			entry.Latitude, _ = strconv.ParseFloat(record[15], 64)
		}
		if record[16] != "" {
			entry.Longtitude, _ = strconv.ParseFloat(record[16], 64)
		}

		// Parse JSON fields
		if record[7] != "" && record[7] != "null" {
			_ = json.Unmarshal([]byte(record[7]), &entry.OpenHours)
		}
		if record[8] != "" && record[8] != "null" {
			_ = json.Unmarshal([]byte(record[8]), &entry.PopularTimes)
		}
		if record[14] != "" && record[14] != "null" {
			_ = json.Unmarshal([]byte(record[14]), &entry.ReviewsPerRating)
		}
		if record[25] != "" && record[25] != "null" {
			_ = json.Unmarshal([]byte(record[25]), &entry.Images)
		}
		if record[26] != "" && record[26] != "null" {
			_ = json.Unmarshal([]byte(record[26]), &entry.Reservations)
		}
		if record[27] != "" && record[27] != "null" {
			_ = json.Unmarshal([]byte(record[27]), &entry.OrderOnline)
		}
		if record[28] != "" && record[28] != "null" {
			_ = json.Unmarshal([]byte(record[28]), &entry.Menu)
		}
		if record[29] != "" && record[29] != "null" {
			_ = json.Unmarshal([]byte(record[29]), &entry.Owner)
		}
		if record[30] != "" && record[30] != "null" {
			_ = json.Unmarshal([]byte(record[30]), &entry.CompleteAddress)
		}
		if record[31] != "" && record[31] != "null" {
			_ = json.Unmarshal([]byte(record[31]), &entry.About)
		}
		if record[32] != "" && record[32] != "null" {
			_ = json.Unmarshal([]byte(record[32]), &entry.UserReviews)
		}
		if record[33] != "" && record[33] != "null" {
			_ = json.Unmarshal([]byte(record[33]), &entry.UserReviewsExtended)
		}
		if record[34] != "" && record[34] != "null" {
			entry.Emails = strings.Split(record[34], ", ")
		}

		results = append(results, entry)
	}

	return results, nil
}
