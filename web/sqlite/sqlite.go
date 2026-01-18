package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite" // sqlite driver

	"github.com/gosom/google-maps-scraper/web"
)

type repo struct {
	db *sql.DB
}

func New(path string) (web.JobRepository, error) {
	db, err := initDatabase(path)
	if err != nil {
		return nil, err
	}

	return &repo{db: db}, nil
}

func NewWithDB(db *sql.DB) web.JobRepository {
	return &repo{db: db}
}

func InitDB(path string) (*sql.DB, error) {
	return initDatabase(path)
}

func (repo *repo) Get(ctx context.Context, id string) (web.Job, error) {
	const q = `SELECT id, name, status, data, organization_id, created_by, created_at, updated_at FROM jobs WHERE id = ?`

	row := repo.db.QueryRowContext(ctx, q, id)

	return rowToJob(row)
}

func (repo *repo) Create(ctx context.Context, job *web.Job) error {
	item, err := jobToRow(job)
	if err != nil {
		return err
	}

	const q = `INSERT INTO jobs (id, name, status, data, organization_id, created_by, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	var orgID, createdBy interface{}
	if item.OrganizationID.Valid {
		orgID = item.OrganizationID.String
	}
	if item.CreatedBy.Valid {
		createdBy = item.CreatedBy.String
	}

	_, err = repo.db.ExecContext(ctx, q, item.ID, item.Name, item.Status, item.Data, orgID, createdBy, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}

	return nil
}

func (repo *repo) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM jobs WHERE id = ?`

	_, err := repo.db.ExecContext(ctx, q, id)

	return err
}

func (repo *repo) Select(ctx context.Context, params web.SelectParams) ([]web.Job, error) {
	q := `SELECT id, name, status, data, organization_id, created_by, created_at, updated_at FROM jobs`

	var args []any
	var conditions []string

	if params.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, params.Status)
	}

	if params.OrganizationID != "" {
		conditions = append(conditions, "organization_id = ?")
		args = append(args, params.OrganizationID)
	}

	if params.CreatedBy != "" {
		conditions = append(conditions, "created_by = ?")
		args = append(args, params.CreatedBy)
	}

	if len(conditions) > 0 {
		q += " WHERE " + conditions[0]
		for i := 1; i < len(conditions); i++ {
			q += " AND " + conditions[i]
		}
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += " LIMIT ?"
		args = append(args, params.Limit)
	}

	rows, err := repo.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var ans []web.Job

	for rows.Next() {
		job, err := rowToJob(rows)
		if err != nil {
			return nil, err
		}

		ans = append(ans, job)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return ans, nil
}

func (repo *repo) Update(ctx context.Context, job *web.Job) error {
	item, err := jobToRow(job)
	if err != nil {
		return err
	}

	const q = `UPDATE jobs SET name = ?, status = ?, data = ?, updated_at = ? WHERE id = ?`

	_, err = repo.db.ExecContext(ctx, q, item.Name, item.Status, item.Data, item.UpdatedAt, item.ID)

	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func rowToJob(row scannable) (web.Job, error) {
	var j job

	err := row.Scan(&j.ID, &j.Name, &j.Status, &j.Data, &j.OrganizationID, &j.CreatedBy, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return web.Job{}, err
	}

	ans := web.Job{
		ID:        j.ID,
		Name:      j.Name,
		Status:    j.Status,
		Date:      time.Unix(j.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(j.UpdatedAt, 0).UTC(),
	}

	if j.OrganizationID.Valid {
		ans.OrganizationID = j.OrganizationID.String
	}
	if j.CreatedBy.Valid {
		ans.CreatedBy = j.CreatedBy.String
	}

	err = json.Unmarshal([]byte(j.Data), &ans.Data)
	if err != nil {
		return web.Job{}, err
	}

	return ans, nil
}

func jobToRow(item *web.Job) (job, error) {
	data, err := json.Marshal(item.Data)
	if err != nil {
		return job{}, err
	}

	row := job{
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

type job struct {
	ID             string
	Name           string
	Status         string
	Data           string
	OrganizationID sql.NullString
	CreatedBy      sql.NullString
	CreatedAt      int64
	UpdatedAt      int64
}

func initDatabase(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)

	_, err = db.Exec("PRAGMA busy_timeout = 5000")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA synchronous=NORMAL")
	if err != nil {
		return nil, err
	}

	_, err = db.Exec("PRAGMA cache_size=1000")
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, createSchema(db)
}

func createSchema(db *sql.DB) error {
	// Create jobs table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			data TEXT NOT NULL,
			created_at INT NOT NULL,
			updated_at INT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	// Create api_keys table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			created_at INT NOT NULL,
			updated_at INT NOT NULL,
			last_used_at INT,
			expires_at INT
		)
	`)
	if err != nil {
		return err
	}

	// Create index on key_hash for faster lookups
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash)
	`)
	if err != nil {
		return err
	}

	// Create index on status for faster filtering
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_api_keys_status ON api_keys(status)
	`)
	if err != nil {
		return err
	}

	// Create users table for authentication
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			first_name TEXT NOT NULL DEFAULT '',
			last_name TEXT NOT NULL DEFAULT '',
			avatar_url TEXT DEFAULT '',
			email_verified INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at INT NOT NULL,
			updated_at INT NOT NULL,
			last_login_at INT,
			deleted_at INT
		)
	`)
	if err != nil {
		return err
	}

	// Create index on email for faster lookups
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)
	`)
	if err != nil {
		return err
	}

	// Create user_sessions table for session management
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS user_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at INT NOT NULL,
			created_at INT NOT NULL,
			last_used_at INT,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return err
	}

	// Create index on token_hash for faster lookups
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_user_sessions_token_hash ON user_sessions(token_hash)
	`)
	if err != nil {
		return err
	}

	// Create index on user_id for faster user session lookups
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id)
	`)
	if err != nil {
		return err
	}

	// Add organization_id and created_by columns to jobs table if they don't exist
	if err := addColumnIfNotExists(db, "jobs", "organization_id", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(db, "jobs", "created_by", "TEXT"); err != nil {
		return err
	}

	// Add organization_id and created_by columns to api_keys table if they don't exist
	if err := addColumnIfNotExists(db, "api_keys", "organization_id", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(db, "api_keys", "created_by", "TEXT"); err != nil {
		return err
	}

	// Create indexes for organization_id columns
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_organization_id ON jobs(organization_id)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_created_by ON jobs(created_by)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_api_keys_organization_id ON api_keys(organization_id)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_api_keys_created_by ON api_keys(created_by)`)
	if err != nil {
		return err
	}

	return nil
}

// addColumnIfNotExists adds a column to a table if it doesn't already exist
func addColumnIfNotExists(db *sql.DB, tableName, columnName, columnType string) error {
	// Check if column exists
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	columnExists := false
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dfltValue interface{}
		var pk int

		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}

		if name == columnName {
			columnExists = true
			break
		}
	}

	if !columnExists {
		_, err = db.Exec("ALTER TABLE " + tableName + " ADD COLUMN " + columnName + " " + columnType)
		return err
	}

	return nil
}
