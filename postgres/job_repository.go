package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

type jobRepository struct {
	db *sql.DB
}

func NewJobRepository(db *sql.DB) web.JobRepository {
	return &jobRepository{db: db}
}

func (r *jobRepository) Get(ctx context.Context, id string) (web.Job, error) {
	const q = `SELECT id, name, status, data, organization_id, created_by, created_at, updated_at FROM jobs WHERE id = $1`

	row := r.db.QueryRowContext(ctx, q, id)

	return rowToJobModel(row)
}

func (r *jobRepository) Create(ctx context.Context, job *web.Job) error {
	item, err := jobModelToRow(job)
	if err != nil {
		return err
	}

	const q = `INSERT INTO jobs (id, name, status, data, organization_id, created_by, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var orgID, createdBy interface{}
	if item.OrganizationID.Valid {
		orgID = item.OrganizationID.String
	}
	if item.CreatedBy.Valid {
		createdBy = item.CreatedBy.String
	}

	_, err = r.db.ExecContext(ctx, q, item.ID, item.Name, item.Status, item.Data, orgID, createdBy, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}

	return nil
}

func (r *jobRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM jobs WHERE id = $1`

	_, err := r.db.ExecContext(ctx, q, id)

	return err
}

func (r *jobRepository) Select(ctx context.Context, params web.SelectParams) ([]web.Job, error) {
	q := `SELECT id, name, status, data, organization_id, created_by, created_at, updated_at FROM jobs`

	var args []any
	var conditions []string
	argNum := 1

	if params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argNum))
		args = append(args, params.Status)
		argNum++
	}

	if params.OrganizationID != "" {
		conditions = append(conditions, fmt.Sprintf("organization_id = $%d", argNum))
		args = append(args, params.OrganizationID)
		argNum++
	}

	if params.CreatedBy != "" {
		conditions = append(conditions, fmt.Sprintf("created_by = $%d", argNum))
		args = append(args, params.CreatedBy)
		argNum++
	}

	if len(conditions) > 0 {
		q += " WHERE " + strings.Join(conditions, " AND ")
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", argNum)
		args = append(args, params.Limit)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var ans []web.Job

	for rows.Next() {
		job, err := rowToJobModel(rows)
		if err != nil {
			return nil, err
		}

		ans = append(ans, job)
	}

	return ans, nil
}

func (r *jobRepository) Update(ctx context.Context, job *web.Job) error {
	item, err := jobModelToRow(job)
	if err != nil {
		return err
	}

	const q = `UPDATE jobs SET name = $1, status = $2, data = $3, updated_at = $4 WHERE id = $5`

	_, err = r.db.ExecContext(ctx, q, item.Name, item.Status, item.Data, item.UpdatedAt, item.ID)

	return err
}

// Helper types and functions

type jobRow struct {
	ID             string
	Name           string
	Status         string
	Data           string
	OrganizationID sql.NullString
	CreatedBy      sql.NullString
	CreatedAt      int64
	UpdatedAt      int64
}

type jobScannable interface {
	Scan(dest ...any) error
}

func rowToJobModel(row jobScannable) (web.Job, error) {
	var item jobRow

	err := row.Scan(&item.ID, &item.Name, &item.Status, &item.Data, &item.OrganizationID, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return web.Job{}, err
	}

	ans := web.Job{
		ID:        item.ID,
		Name:      item.Name,
		Status:    item.Status,
		Date:      time.Unix(item.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(item.UpdatedAt, 0).UTC(),
	}

	if item.OrganizationID.Valid {
		ans.OrganizationID = item.OrganizationID.String
	}
	if item.CreatedBy.Valid {
		ans.CreatedBy = item.CreatedBy.String
	}

	err = json.Unmarshal([]byte(item.Data), &ans.Data)
	if err != nil {
		return web.Job{}, err
	}

	return ans, nil
}

func jobModelToRow(item *web.Job) (jobRow, error) {
	data, err := json.Marshal(item.Data)
	if err != nil {
		return jobRow{}, err
	}

	row := jobRow{
		ID:        item.ID,
		Name:      item.Name,
		Status:    item.Status,
		Data:      string(data),
		CreatedAt: item.Date.Unix(),
		UpdatedAt: time.Now().UTC().Unix(),
	}

	if item.OrganizationID != "" {
		row.OrganizationID = sql.NullString{String: item.OrganizationID, Valid: true}
	}
	if item.CreatedBy != "" {
		row.CreatedBy = sql.NullString{String: item.CreatedBy, Valid: true}
	}

	return row, nil
}
