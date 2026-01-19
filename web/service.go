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
	// Extract user and organization from context
	user := getUserFromContext(ctx)
	orgID := getOrganizationIDFromContext(ctx)

	if orgID == "" {
		return fmt.Errorf("organization context required")
	}

	// Populate organization_id and created_by
	job.OrganizationID = orgID
	if user != nil {
		job.CreatedBy = user.ID
	}

	return s.repo.Create(ctx, job)
}

func (s *Service) All(ctx context.Context) ([]Job, error) {
	// Extract organization from context
	orgID := getOrganizationIDFromContext(ctx)
	if orgID == "" {
		return nil, fmt.Errorf("organization context required")
	}

	return s.repo.Select(ctx, SelectParams{OrganizationID: orgID})
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	job, err := s.repo.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}

	// Verify organization access
	orgID := getOrganizationIDFromContext(ctx)
	if orgID != "" && job.OrganizationID != orgID {
		return Job{}, fmt.Errorf("job not found") // Don't leak existence
	}

	return job, nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	// Verify ownership first
	_, err := s.Get(ctx, id) // Uses ownership check
	if err != nil {
		return err
	}

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
	// Verify ownership
	existing, err := s.Get(ctx, job.ID) // Uses ownership check
	if err != nil {
		return err
	}

	// Preserve organization_id and created_by
	job.OrganizationID = existing.OrganizationID
	job.CreatedBy = existing.CreatedBy

	return s.repo.Update(ctx, job)
}

func (s *Service) SelectPending(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusPending, Limit: 1})
}

func (s *Service) SelectWorking(ctx context.Context) ([]Job, error) {
	return s.repo.Select(ctx, SelectParams{Status: StatusWorking})
}

func (s *Service) Retry(ctx context.Context, id string) error {
	// Get the job (with ownership check)
	job, err := s.Get(ctx, id)
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
		// If file is empty or only has BOM, return empty array instead of error
		if err.Error() == "EOF" {
			return []gmaps.Entry{}, nil
		}
		return nil, fmt.Errorf("failed to read csv header: %w", err)
	}

	// Initialize as empty slice instead of nil to ensure JSON marshals to [] not null
	results := make([]gmaps.Entry, 0)

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

		// Parse based on CsvHeaders() order:
		// 0:input_id, 1:link, 2:title, 3:category, 4:address, 5:open_hours, 6:popular_times,
		// 7:website, 8:phone, 9:plus_code, 10:review_count, 11:review_rating, 12:reviews_per_rating,
		// 13:latitude, 14:longitude, 15:cid, 16:status, 17:descriptions, 18:reviews_link,
		// 19:thumbnail, 20:timezone, 21:price_range, 22:data_id, 23:place_id, 24:place_id_url,
		// 25:images, 26:reservations, 27:order_online, 28:menu, 29:owner, 30:complete_address,
		// 31:about, 32:user_reviews, 33:user_reviews_extended, 34:emails
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
			PlaceID:     record[23],
			PlaceIDURL:  record[24],
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
		if record[5] != "" && record[5] != "null" && record[5] != "{}" {
			_ = json.Unmarshal([]byte(record[5]), &entry.OpenHours)
		}
		if record[6] != "" && record[6] != "null" && record[6] != "{}" {
			_ = json.Unmarshal([]byte(record[6]), &entry.PopularTimes)
		}
		if record[12] != "" && record[12] != "null" {
			_ = json.Unmarshal([]byte(record[12]), &entry.ReviewsPerRating)
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
	// Extract user and organization from context
	user := getUserFromContext(ctx)
	orgID := getOrganizationIDFromContext(ctx)

	if orgID == "" {
		return nil, fmt.Errorf("organization context required")
	}

	// Create a new job for the imported data
	jobID := uuid.New()
	now := time.Now()
	job := &Job{
		ID:             jobID.String(),
		OrganizationID: orgID,
		Name:           jobName,
		Status:         StatusOK,
		Date:           now,
		UpdatedAt:      now,
	}

	if user != nil {
		job.CreatedBy = user.ID
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
	// Extract user and organization from context
	user := getUserFromContext(ctx)
	orgID := getOrganizationIDFromContext(ctx)

	if orgID == "" {
		return nil, fmt.Errorf("organization context required")
	}

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
		ID:             jobID.String(),
		OrganizationID: orgID,
		Name:           jobName,
		Status:         StatusOK,
		Date:           now,
		UpdatedAt:      now,
	}

	if user != nil {
		job.CreatedBy = user.ID
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
