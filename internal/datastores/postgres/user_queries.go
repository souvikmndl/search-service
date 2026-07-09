package postgres

import (
	"context"

	"github.com/souvikmndl/search-service/internal/model"
)

// UserDB exposes user db queries
type UserDB interface {
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
	GetUserByEmail(ctx context.Context, emailID string) (*model.User, error)
}

// CreateUser creates a new user row
func (db *DB) CreateUser(ctx context.Context, user *model.User) error {
	query := `
	INSERT INTO users (
	    email_id,
	    user_name,
	    password
	)
	VALUES ($1,$2,$3)
	RETURNING id, created_at;
	`

	return db.conn.QueryRowContext(
		ctx,
		query,
		user.EmailID,
		user.UserName,
		user.Password,
	).Scan(
		&user.ID,
		&user.CreatedAt,
	)
}

// GetUserByID fetches an user by id
func (db *DB) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	query := `
	SELECT
	    id,
	    email_id,
	    user_name,
	    password,
	    created_at
	FROM users
	WHERE id=$1;
	`

	user := model.User{}
	err := db.conn.QueryRowContext(
		ctx,
		query,
		id,
	).Scan(
		&user.ID,
		&user.EmailID,
		&user.UserName,
		&user.Password,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

// GetUserByEmail fetches a user by email.
func (db *DB) GetUserByEmail(ctx context.Context, emailID string) (*model.User, error) {
	query := `
	SELECT
	    id,
	    email_id,
	    user_name,
	    password,
	    created_at
	FROM users
	WHERE email_id=$1;
	`

	user := model.User{}
	err := db.conn.QueryRowContext(
		ctx,
		query,
		emailID,
	).Scan(
		&user.ID,
		&user.EmailID,
		&user.UserName,
		&user.Password,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
