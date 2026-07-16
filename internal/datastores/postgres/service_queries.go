package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/souvikmndl/search-service/internal/model"
)

// ServiceDB exposes all service db queries
type ServiceDB interface {
	SearchServices(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error)
	CreateService(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error)
	GetServiceByID(ctx context.Context, id int64) (*model.Service, error)
	CreateServiceVersion(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error)
	GetServiceVersions(ctx context.Context, serviceID int64) ([]model.Version, error)
	SearchVersions(ctx context.Context, svcID int64, params model.VersionSearchParams) ([]model.Version, int64, error)
	UpdateService(ctx context.Context, service *model.Service) error
	DeleteServiceByID(ctx context.Context, id int64, changedBy int64) error
	GetAuditLog(ctx context.Context, serviceID int64) ([]model.AuditEntry, error)
}

// tagsSubquery returns a correlated subquery that produces a JSON array of tag
// names for a given service row aliased as "s". The COALESCE ensures an empty
// array is returned instead of NULL when the service has no tags.
const tagsSubquery = `
	COALESCE(
		(SELECT json_agg(t.name ORDER BY t.name)
		 FROM   service_tags st
		 JOIN   tags t ON t.id = st.tag_id
		 WHERE  st.service_id = s.id),
		'[]'::json
	) AS tags`

// scanTags unmarshals a raw JSON byte slice (e.g. ["api","payments"]) from
// the tagsSubquery into a Go string slice.
func scanTags(raw []byte) ([]string, error) {
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return nil, fmt.Errorf("unmarshal tags: %w", err)
	}
	if tags == nil {
		tags = []string{}
	}
	return tags, nil
}

// upsertServiceTags replaces all tags for a service inside an open transaction.
// It upserts each tag name into the tags table, then re-creates the
// service_tags rows from scratch (delete-then-insert is safe here because the
// whole operation is wrapped in the caller's transaction).
func upsertServiceTags(ctx context.Context, tx *sql.Tx, serviceID int64, tags []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_tags WHERE service_id = $1`, serviceID); err != nil {
		return fmt.Errorf("clear service tags: %w", err)
	}

	for _, name := range tags {
		var tagID int64
		err := tx.QueryRowContext(ctx,
			`INSERT INTO tags (name) VALUES ($1)
			 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			 RETURNING id`,
			name,
		).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("upsert tag %q: %w", name, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO service_tags (service_id, tag_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			serviceID, tagID,
		); err != nil {
			return fmt.Errorf("link tag %q: %w", name, err)
		}
	}

	return nil
}

// insertAuditLog writes a single audit row inside an open transaction.
// payload must already be marshalled to JSON.
func insertAuditLog(ctx context.Context, tx *sql.Tx, serviceID int64, action string, changedBy int64, payload []byte) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO service_audit_log (service_id, action, changed_by, payload)
		 VALUES ($1, $2, $3, $4)`,
		serviceID, action, changedBy, payload,
	)
	return err
}

// SearchServices filters services based on optional params
func (db *DB) SearchServices(
	ctx context.Context,
	params model.ServiceSearchParams,
) ([]model.Service, int64, error) {
	var conditions []string
	var args []any
	argID := 1

	if params.Name != "" {
		conditions = append(conditions, fmt.Sprintf("s.name ILIKE '%%' || $%d || '%%'", argID))
		args = append(args, params.Name)
		argID++
	}

	if params.Description != "" {
		conditions = append(conditions, fmt.Sprintf("s.search_vector @@ plainto_tsquery('english', $%d)", argID))
		args = append(args, params.Description)
		argID++
	}

	if params.Tag != "" {
		// EXISTS subquery keeps the outer COUNT(sv.id) aggregation correct —
		// adding tag JOINs to the outer query would require extra GROUP BY columns.
		conditions = append(conditions, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM service_tags st JOIN tags t ON t.id = st.tag_id WHERE st.service_id = s.id AND t.name = $%d)`,
			argID,
		))
		args = append(args, params.Tag)
		argID++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	sortBy := "created_at"
	if strings.ToLower(params.SortBy) == "name" {
		sortBy = "name"
	}
	order := "ASC"
	if strings.ToUpper(params.Order) == "DESC" {
		order = "DESC"
	}

	if params.PageSize < 10 || params.PageSize > 20 {
		params.PageSize = 10
	}
	if params.Page < 1 {
		params.Page = 1
	}
	offset := (params.Page - 1) * params.PageSize

	// COUNT(*) OVER() is a window function evaluated after GROUP BY but before
	// LIMIT, so it returns the total number of matching services regardless of
	// the requested page.
	query := fmt.Sprintf(`
		SELECT
			s.id,
			s.name,
			s.description,
			COUNT(sv.id) AS number_of_versions,
			s.created_by,
			s.updated_by,
			s.created_at,
			s.updated_at,
			COUNT(*) OVER() AS total_count,
			%s
		FROM services s
		LEFT JOIN service_versions sv ON sv.service_id = s.id
		%s
		GROUP BY s.id
		ORDER BY s.%s %s
		LIMIT $%d OFFSET $%d`,
		tagsSubquery, whereClause, sortBy, order, argID, argID+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}
	defer rows.Close()

	var services []model.Service
	var total int64
	for rows.Next() {
		var s model.Service
		var tagsRaw []byte
		if err := rows.Scan(
			&s.ID,
			&s.Name,
			&s.Description,
			&s.NumberOfVersions,
			&s.CreatedBy,
			&s.UpdatedBy,
			&s.CreatedAt,
			&s.UpdatedAt,
			&total,
			&tagsRaw,
		); err != nil {
			return nil, 0, fmt.Errorf("search scan: %w", err)
		}
		if s.Tags, err = scanTags(tagsRaw); err != nil {
			return nil, 0, err
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search rows: %w", err)
	}
	return services, total, nil
}

// SearchVersions filters versions for a service with optional status/creator filters.
func (db *DB) SearchVersions(ctx context.Context, svcID int64, params model.VersionSearchParams) ([]model.Version, int64, error) {
	var conditions []string
	var args []any
	argID := 1

	conditions = append(conditions, fmt.Sprintf("service_id = $%d", argID))
	args = append(args, svcID)
	argID++

	if params.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argID))
		args = append(args, params.Status)
		argID++
	}

	if params.CreatedBy > 0 {
		conditions = append(conditions, fmt.Sprintf("created_by = $%d", argID))
		args = append(args, params.CreatedBy)
		argID++
	}

	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 10 || params.PageSize > 20 {
		params.PageSize = 10
	}
	offset := (params.Page - 1) * params.PageSize

	query := fmt.Sprintf(`
		SELECT
			id,
			version_string,
			service_id,
			status,
			created_by,
			created_at,
			COUNT(*) OVER() AS total_count
		FROM service_versions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(conditions, " AND "), argID, argID+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search versions: %w", err)
	}
	defer rows.Close()

	var versions []model.Version
	var total int64
	for rows.Next() {
		var v model.Version
		if err := rows.Scan(&v.ID, &v.VersionString, &v.ServiceID, &v.Status, &v.CreatedBy, &v.CreatedAt, &total); err != nil {
			return nil, 0, fmt.Errorf("search versions scan: %w", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search versions rows: %w", err)
	}
	return versions, total, nil
}

// CreateService creates a service and its first version atomically.
func (db *DB) CreateService(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	service := model.Service{}
	err = tx.QueryRowContext(
		ctx,
		`INSERT INTO services (name, description, created_by, updated_by)
		 VALUES ($1, $2, $3, $3)
		 RETURNING id, name, description, created_by, updated_by, created_at, updated_at`,
		req.Name,
		req.Description,
		req.CreatedBy,
	).Scan(
		&service.ID,
		&service.Name,
		&service.Description,
		&service.CreatedBy,
		&service.UpdatedBy,
		&service.CreatedAt,
		&service.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO service_versions (service_id, version_string, status, created_by)
		 VALUES ($1, $2, $3, $4)`,
		service.ID,
		req.Version.VersionString,
		req.Version.Status,
		req.CreatedBy,
	); err != nil {
		return nil, err
	}

	if err := upsertServiceTags(ctx, tx, service.ID, req.Tags); err != nil {
		return nil, err
	}

	payload, _ := json.Marshal(map[string]any{
		"name":        service.Name,
		"description": service.Description,
		"tags":        req.Tags,
	})
	if err := insertAuditLog(ctx, tx, service.ID, "create", req.CreatedBy, payload); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	service.NumberOfVersions = 1
	service.Tags = req.Tags
	if service.Tags == nil {
		service.Tags = []string{}
	}
	return &service, nil
}

// GetServiceByID fetches a service by id
func (db *DB) GetServiceByID(ctx context.Context, id int64) (*model.Service, error) {
	query := fmt.Sprintf(`
		SELECT
			s.id,
			s.name,
			s.description,
			COUNT(sv.id) AS number_of_versions,
			s.created_by,
			s.updated_by,
			s.created_at,
			s.updated_at,
			%s
		FROM services s
		LEFT JOIN service_versions sv ON sv.service_id = s.id
		WHERE s.id = $1
		GROUP BY s.id`, tagsSubquery)

	var s model.Service
	var tagsRaw []byte
	err := db.conn.QueryRowContext(ctx, query, id).Scan(
		&s.ID,
		&s.Name,
		&s.Description,
		&s.NumberOfVersions,
		&s.CreatedBy,
		&s.UpdatedBy,
		&s.CreatedAt,
		&s.UpdatedAt,
		&tagsRaw,
	)
	if err != nil {
		return nil, err
	}
	var scanErr error
	if s.Tags, scanErr = scanTags(tagsRaw); scanErr != nil {
		return nil, scanErr
	}
	return &s, nil
}

// CreateServiceVersion creates a new version entry for an existing service
func (db *DB) CreateServiceVersion(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error) {
	var v model.Version
	err := db.conn.QueryRowContext(ctx,
		`INSERT INTO service_versions (service_id, version_string, status, created_by)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, service_id, version_string, status, created_by, created_at`,
		serviceID,
		version.VersionString,
		version.Status,
		version.CreatedBy,
	).Scan(&v.ID, &v.ServiceID, &v.VersionString, &v.Status, &v.CreatedBy, &v.CreatedAt)
	return &v, err
}

// GetServiceVersions fetches versions for a service by id.
func (db *DB) GetServiceVersions(ctx context.Context, serviceID int64) ([]model.Version, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, service_id, version_string, status, created_by, created_at
		 FROM service_versions
		 WHERE service_id = $1
		 ORDER BY created_at DESC, id DESC`,
		serviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.Version
	for rows.Next() {
		var v model.Version
		if err := rows.Scan(&v.ID, &v.ServiceID, &v.VersionString, &v.Status, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// UpdateService updates name, description, and tags of an existing service.
func (db *DB) UpdateService(ctx context.Context, service *model.Service) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := tx.QueryRowContext(ctx,
		`UPDATE services
		 SET name = $1, description = $2, updated_by = $3, updated_at = NOW()
		 WHERE id = $4
		 RETURNING updated_by, updated_at`,
		service.Name,
		service.Description,
		service.UpdatedBy,
		service.ID,
	).Scan(&service.UpdatedBy, &service.UpdatedAt); err != nil {
		return err
	}

	if err := upsertServiceTags(ctx, tx, service.ID, service.Tags); err != nil {
		return err
	}

	payload, _ := json.Marshal(map[string]any{
		"name":        service.Name,
		"description": service.Description,
		"tags":        service.Tags,
	})
	if err := insertAuditLog(ctx, tx, service.ID, "update", service.UpdatedBy, payload); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteServiceByID deletes a service by id and records an audit entry.
func (db *DB) DeleteServiceByID(ctx context.Context, id int64, changedBy int64) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var name string
	if err := tx.QueryRowContext(ctx, `SELECT name FROM services WHERE id = $1`, id).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return sql.ErrNoRows
		}
		return err
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM services WHERE id = $1`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	payload, _ := json.Marshal(map[string]any{"id": id, "name": name})
	if err := insertAuditLog(ctx, tx, id, "delete", changedBy, payload); err != nil {
		return err
	}

	return tx.Commit()
}

// GetAuditLog returns the audit history for a service, newest first.
func (db *DB) GetAuditLog(ctx context.Context, serviceID int64) ([]model.AuditEntry, error) {
	rows, err := db.conn.QueryContext(ctx,
		`SELECT id, service_id, action, changed_by, changed_at, payload
		 FROM service_audit_log
		 WHERE service_id = $1
		 ORDER BY changed_at DESC`,
		serviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}
	defer rows.Close()

	var entries []model.AuditEntry
	for rows.Next() {
		var e model.AuditEntry
		if err := rows.Scan(&e.ID, &e.ServiceID, &e.Action, &e.ChangedBy, &e.ChangedAt, &e.Payload); err != nil {
			return nil, fmt.Errorf("audit log scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit log rows: %w", err)
	}
	return entries, nil
}
