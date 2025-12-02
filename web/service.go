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
	"time"

	"github.com/google/uuid"
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

// HasResults checks if a job has results in its CSV file (more than just the header)
func (s *Service) HasResults(id string) bool {
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return false
	}

	datapath := filepath.Join(s.dataFolder, id+".csv")

	// Check if file exists
	fileInfo, err := os.Stat(datapath)
	if err != nil {
		return false
	}

	// Check if file has content (more than just BOM + header, roughly > 500 bytes)
	if fileInfo.Size() < 500 {
		return false
	}

	// Quick check: try to read at least 2 rows (header + 1 data row)
	file, err := os.Open(datapath)
	if err != nil {
		return false
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	_, err = reader.Read()
	if err != nil {
		return false
	}

	// Try to read at least one data row
	_, err = reader.Read()
	return err == nil // If we can read a data row, job has results
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

func (s *Service) ImportFromCSV(ctx context.Context, jobName string, csvData []byte) (*Job, error) {
	// Create a new job for the imported data
	jobID := uuid.New()
	now := time.Now()
	job := &Job{
		ID:        jobID.String(),
		Name:      jobName,
		Status:    StatusOK,
		Date:      now,
		UpdatedAt: now,
	}

	// Parse CSV to validate
	reader := csv.NewReader(strings.NewReader(string(csvData)))

	// Read and validate header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv header: %w", err)
	}

	// Expected headers
	var entry gmaps.Entry
	expectedHeaders := entry.CsvHeaders()
	if len(header) != len(expectedHeaders) {
		return nil, fmt.Errorf("invalid csv format: expected %d columns, got %d", len(expectedHeaders), len(header))
	}

	// Validate that the file has the required columns (at least place_id or place_id_url or link)
	hasRequiredFields := false
	for i, h := range header {
		if i < len(expectedHeaders) && h == expectedHeaders[i] {
			if h == "place_id" || h == "place_id_url" || h == "link" {
				hasRequiredFields = true
			}
		}
	}

	if !hasRequiredFields {
		return nil, fmt.Errorf("csv must contain at least one of: place_id, place_id_url, or link")
	}

	// Write the CSV to the data folder
	datapath := filepath.Join(s.dataFolder, jobID.String()+".csv")

	// Add UTF-8 BOM for Excel compatibility
	file, err := os.Create(datapath)
	if err != nil {
		return nil, fmt.Errorf("failed to create csv file: %w", err)
	}
	defer file.Close()

	// Write BOM
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := file.Write(bom); err != nil {
		return nil, fmt.Errorf("failed to write BOM: %w", err)
	}

	// Write CSV data
	if _, err := file.Write(csvData); err != nil {
		return nil, fmt.Errorf("failed to write csv data: %w", err)
	}

	// Create the job in the database
	if err := s.repo.Create(ctx, job); err != nil {
		// Clean up the file if job creation fails
		os.Remove(datapath)
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}

func (s *Service) ImportFromJSON(ctx context.Context, jobName string, jsonData []byte) (*Job, error) {
	// Parse JSON to validate
	var entries []gmaps.Entry
	if err := json.Unmarshal(jsonData, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse json: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("json file contains no entries")
	}

	// Validate entries have required fields
	for i, entry := range entries {
		if entry.Link == "" && entry.PlaceID == "" && entry.PlaceIDURL == "" {
			return nil, fmt.Errorf("entry %d missing required fields (must have link, place_id, or place_id_url)", i)
		}
	}

	// Create a new job for the imported data
	jobID := uuid.New()
	now := time.Now()
	job := &Job{
		ID:        jobID.String(),
		Name:      jobName,
		Status:    StatusOK,
		Date:      now,
		UpdatedAt: now,
	}

	// Write entries to CSV file
	datapath := filepath.Join(s.dataFolder, jobID.String()+".csv")

	file, err := os.Create(datapath)
	if err != nil {
		return nil, fmt.Errorf("failed to create csv file: %w", err)
	}
	defer file.Close()

	// Write UTF-8 BOM for Excel compatibility
	bom := []byte{0xEF, 0xBB, 0xBF}
	if _, err := file.Write(bom); err != nil {
		return nil, fmt.Errorf("failed to write BOM: %w", err)
	}

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write(entries[0].CsvHeaders()); err != nil {
		return nil, fmt.Errorf("failed to write csv header: %w", err)
	}

	// Write entries
	for _, entry := range entries {
		if err := writer.Write(entry.CsvRow()); err != nil {
			return nil, fmt.Errorf("failed to write csv row: %w", err)
		}
	}

	// Create the job in the database
	if err := s.repo.Create(ctx, job); err != nil {
		// Clean up the file if job creation fails
		os.Remove(datapath)
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	return job, nil
}
