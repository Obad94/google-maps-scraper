package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gosom/google-maps-scraper/web"
)

var _ web.AuditLogRepository = (*auditLogRepository)(nil)

type auditLogRepository struct {
	db *sql.DB
}

func NewAuditLogRepository(db *sql.DB) web.AuditLogRepository {
	return &auditLogRepository{db: db}
}

func (r *auditLogRepository) Get(ctx context.Context, id string) (web.AuditLog, error) {
	q := `
		SELECT id, organization_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE id = $1
	`

	var log web.AuditLog
	var orgID, userID, resourceID sql.NullString
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&log.ID,
		&orgID,
		&userID,
		&log.Action,
		&log.ResourceType,
		&resourceID,
		&metadataJSON,
		&log.IPAddress,
		&log.UserAgent,
		&log.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return web.AuditLog{}, fmt.Errorf("audit log not found: %w", err)
		}
		return web.AuditLog{}, fmt.Errorf("failed to get audit log: %w", err)
	}

	if orgID.Valid {
		log.OrganizationID = &orgID.String
	}

	if userID.Valid {
		log.UserID = &userID.String
	}

	if resourceID.Valid {
		log.ResourceID = resourceID.String
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &log.Metadata); err != nil {
			return web.AuditLog{}, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return log, nil
}

func (r *auditLogRepository) Create(ctx context.Context, log *web.AuditLog) error {
	if err := log.Validate(); err != nil {
		return fmt.Errorf("invalid audit log: %w", err)
	}

	metadataJSON, err := json.Marshal(log.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	q := `
		INSERT INTO audit_logs (id, organization_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err = r.db.ExecContext(ctx, q,
		log.ID,
		log.OrganizationID,
		log.UserID,
		log.Action,
		log.ResourceType,
		log.ResourceID,
		metadataJSON,
		log.IPAddress,
		log.UserAgent,
		log.CreatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to create audit log: %w", err)
	}

	return nil
}

func (r *auditLogRepository) Select(ctx context.Context, params web.AuditLogSelectParams) ([]web.AuditLog, error) {
	q := `
		SELECT id, organization_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE 1=1
	`

	args := []interface{}{}
	argCount := 1

	if params.OrganizationID != nil {
		q += fmt.Sprintf(" AND organization_id = $%d", argCount)
		args = append(args, *params.OrganizationID)
		argCount++
	}

	if params.UserID != nil {
		q += fmt.Sprintf(" AND user_id = $%d", argCount)
		args = append(args, *params.UserID)
		argCount++
	}

	if params.Action != "" {
		q += fmt.Sprintf(" AND action = $%d", argCount)
		args = append(args, params.Action)
		argCount++
	}

	if params.ResourceType != "" {
		q += fmt.Sprintf(" AND resource_type = $%d", argCount)
		args = append(args, params.ResourceType)
		argCount++
	}

	if params.ResourceID != "" {
		q += fmt.Sprintf(" AND resource_id = $%d", argCount)
		args = append(args, params.ResourceID)
		argCount++
	}

	if params.StartDate != nil {
		q += fmt.Sprintf(" AND created_at >= $%d", argCount)
		args = append(args, *params.StartDate)
		argCount++
	}

	if params.EndDate != nil {
		q += fmt.Sprintf(" AND created_at <= $%d", argCount)
		args = append(args, *params.EndDate)
		argCount++
	}

	q += " ORDER BY created_at DESC"

	if params.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", argCount)
		args = append(args, params.Limit)
		argCount++
	}

	if params.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", argCount)
		args = append(args, params.Offset)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to select audit logs: %w", err)
	}
	defer rows.Close()

	var logs []web.AuditLog

	for rows.Next() {
		var log web.AuditLog
		var orgID, userID, resourceID sql.NullString
		var metadataJSON []byte

		err := rows.Scan(
			&log.ID,
			&orgID,
			&userID,
			&log.Action,
			&log.ResourceType,
			&resourceID,
			&metadataJSON,
			&log.IPAddress,
			&log.UserAgent,
			&log.CreatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan audit log: %w", err)
		}

		if orgID.Valid {
			log.OrganizationID = &orgID.String
		}

		if userID.Valid {
			log.UserID = &userID.String
		}

		if resourceID.Valid {
			log.ResourceID = resourceID.String
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &log.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate audit logs: %w", err)
	}

	return logs, nil
}

func (r *auditLogRepository) DeleteOldLogs(ctx context.Context, before time.Time) error {
	q := `DELETE FROM audit_logs WHERE created_at < $1`

	_, err := r.db.ExecContext(ctx, q, before)
	if err != nil {
		return fmt.Errorf("failed to delete old logs: %w", err)
	}

	return nil
}
