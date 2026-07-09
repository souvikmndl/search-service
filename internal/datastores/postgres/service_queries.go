package postgres

import (
	"context"
	"database/sql"
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
	UpdateService(ctx context.Context, service *model.Service) error
	DeleteServiceByID(ctx context.Context, id int64) error
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
			COUNT(*) OVER() AS total_count
		FROM services s
		LEFT JOIN service_versions sv ON sv.service_id = s.id
		%s
		GROUP BY s.id
		ORDER BY s.%s %s
		LIMIT $%d OFFSET $%d`,
		whereClause, sortBy, order, argID, argID+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %v", err)
	}
	defer rows.Close()

	var services []model.Service
	var total int64
	for rows.Next() {
		var s model.Service
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
		); err != nil {
			return nil, 0, fmt.Errorf("search scan: %v", err)
		}
		services = append(services, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("search rows: %v", err)
	}
	return services, total, nil
}

// CreateService creates a service and its first version.
func (db *DB) CreateService(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error) {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	service := model.Service{}
	err = tx.QueryRowContext(
		ctx,
		`
		INSERT INTO services (
			name,
			description,
			created_by,
			updated_by
		)
		VALUES ($1, $2, $3, $3)
		RETURNING
			id,
			name,
			description,
			created_by,
			updated_by,
			created_at,
			updated_at;
		`,
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

	_, err = tx.ExecContext(
		ctx,
		`
		INSERT INTO service_versions (
			service_id,
			version_string,
			status,
			created_by
		)
		VALUES ($1, $2, $3, $4);
		`,
		service.ID,
		req.Version.VersionString,
		req.Version.Status,
		req.CreatedBy,
	)
	if err != nil {
		return nil, err
	}

	service.NumberOfVersions = 1

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &service, nil
}

// GetServiceByID fetches a service by id
func (db *DB) GetServiceByID(ctx context.Context, id int64) (*model.Service, error) {
	query := `
	SELECT
		s.id,
		s.name,
		s.description,
		COUNT(sv.id) AS number_of_versions,
		s.created_by,
		s.updated_by,
		s.created_at,
		s.updated_at
	FROM services s
	LEFT JOIN service_versions sv ON sv.service_id = s.id
	WHERE s.id = $1
	GROUP BY s.id;
	`

	service := model.Service{}
	err := db.conn.QueryRowContext(
		ctx,
		query,
		id,
	).Scan(
		&service.ID,
		&service.Name,
		&service.Description,
		&service.NumberOfVersions,
		&service.CreatedBy,
		&service.UpdatedBy,
		&service.CreatedAt,
		&service.UpdatedAt,
	)

	return &service, err
}

// CreateServiceVersion creates a new version entry for an existing service
func (db *DB) CreateServiceVersion(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error) {
	var serviceVersion model.Version
	err := db.conn.QueryRowContext(
		ctx,
		`INSERT INTO service_versions(
			service_id,
			version_string,
			status,
			created_by
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id,
			service_id,
			version_string,
			status,
			created_by,
			created_at;
		`,
		serviceID,
		version.VersionString,
		version.Status,
		version.CreatedBy,
	).Scan(
		&serviceVersion.ID,
		&serviceVersion.ServiceID,
		&serviceVersion.VersionString,
		&serviceVersion.Status,
		&serviceVersion.CreatedBy,
		&serviceVersion.CreatedAt,
	)

	return &serviceVersion, err
}

// GetServiceVersions fetches versions for a service by id.
func (db *DB) GetServiceVersions(ctx context.Context, serviceID int64) ([]model.Version, error) {
	rows, err := db.conn.QueryContext(
		ctx,
		`
		SELECT id, service_id, version_string, status, created_by, created_at
		FROM service_versions
		WHERE service_id = $1
		ORDER BY created_at DESC, id DESC;
		`,
		serviceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.Version
	for rows.Next() {
		var version model.Version
		if err := rows.Scan(
			&version.ID,
			&version.ServiceID,
			&version.VersionString,
			&version.Status,
			&version.CreatedBy,
			&version.CreatedAt,
		); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return versions, nil
}

// UpdateService updates an existing service in db
func (db *DB) UpdateService(ctx context.Context, service *model.Service) error {
	query := `
	UPDATE services
	SET
		name = $1,
		description = $2,
		updated_by = $3,
		updated_at = NOW()
	WHERE id = $4
	RETURNING
		updated_by,
		updated_at;
	`

	return db.conn.QueryRowContext(
		ctx,
		query,
		service.Name,
		service.Description,
		service.UpdatedBy,
		service.ID,
	).Scan(
		&service.UpdatedBy,
		&service.UpdatedAt,
	)
}

// DeleteServiceByID deletes a service by id
func (db *DB) DeleteServiceByID(ctx context.Context, id int64) error {
	result, err := db.conn.ExecContext(
		ctx,
		`DELETE FROM services WHERE id = $1`,
		id,
	)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

/*
SELECT s.id, s.name, s.description, COUNT(sv.id) AS number_of_versions, s.created_at, s.updated_at FROM services s LEFT JOIN service_versions sv ON sv.service_id = s.id WHERE s.search_vector @@ plainto_tsquery('english', $1) GROUP BY s.id ORDER BY s.name ASC LIMIT $2 OFFSET $3
*/
