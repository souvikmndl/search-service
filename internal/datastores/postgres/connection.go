package postgres

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // driver for psql
	"github.com/souvikmndl/search-service/internal/config"
)

// SearchDB exposes all db queries
type SearchDB interface {
	UserDB
	ServiceDB
}

// DB holds the postgres db connection
type DB struct {
	conn *sql.DB
}

// NewDB creates and returns a new *DB
func NewDB(conn *sql.DB) *DB {
	return &DB{
		conn: conn,
	}
}

// NewDBConnection connects to db, pings for test and returns pointer to connection
func NewDBConnection(cfg *config.Config) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.DB.DatabaseURL())
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}
