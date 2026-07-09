//go:build integration

package postgres

// Integration tests for the postgres layer.
//
// These tests require a live PostgreSQL database with the schema already applied
// (run the app once with docker-compose so migrations execute, or apply
// migrations/000001_releaseV1.up.sql manually).
//
// Set TEST_DATABASE_URL before running:
//
//	TEST_DATABASE_URL="postgresql://postgres:secret@localhost:5432/search_service?sslmode=disable" \
//	    go test ./internal/datastores/postgres/... -v -run Integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/souvikmndl/search-service/internal/model"
)

// setupIntegrationDB connects to the test DB and registers a cleanup that
// truncates all test tables so each test starts from a clean slate.
func setupIntegrationDB(t *testing.T) *DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration tests")
	}

	conn, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		t.Fatalf("ping db: %v", err)
	}

	t.Cleanup(func() {
		conn.ExecContext(context.Background(),
			`TRUNCATE service_versions, services, users RESTART IDENTITY CASCADE`)
		conn.Close()
	})

	return NewDB(conn)
}

// seedUser creates a test user and returns it with the DB-assigned ID.
func seedUser(t *testing.T, db *DB, email, username string) *model.User {
	t.Helper()
	u := &model.User{EmailID: email, UserName: username, Password: "hashed"}
	if err := db.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return u
}

func TestIntegrationCreateAndGetUser(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	u := &model.User{EmailID: "alice@example.com", UserName: "alice", Password: "hashed"}
	if err := db.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected non-zero ID after CreateUser")
	}
	if u.CreatedAt.IsZero() {
		t.Fatal("expected non-zero CreatedAt after CreateUser")
	}

	got, err := db.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.EmailID != "alice@example.com" || got.UserName != "alice" {
		t.Fatalf("unexpected user: %+v", got)
	}
}

func TestIntegrationGetUserByEmail(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	seedUser(t, db, "bob@example.com", "bob")

	got, err := db.GetUserByEmail(ctx, "bob@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.UserName != "bob" {
		t.Fatalf("expected username bob, got %s", got.UserName)
	}

	_, err = db.GetUserByEmail(ctx, "nobody@example.com")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows for unknown email, got %v", err)
	}
}

func TestIntegrationCreateAndGetService(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "carol@example.com", "carol")

	req := model.CreateServiceRequest{
		Name:        "My Service",
		Description: "a great service",
		Version:     model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
		CreatedBy:   owner.ID,
	}
	svc, err := db.CreateService(ctx, req)
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if svc.ID == 0 {
		t.Fatal("expected non-zero service ID")
	}
	if svc.NumberOfVersions != 1 {
		t.Fatalf("expected 1 version after creation, got %d", svc.NumberOfVersions)
	}

	fetched, err := db.GetServiceByID(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetServiceByID: %v", err)
	}
	if fetched.Name != "My Service" || fetched.Description != "a great service" {
		t.Fatalf("unexpected service: %+v", fetched)
	}
	if fetched.NumberOfVersions != 1 {
		t.Fatalf("expected 1 version from GetServiceByID, got %d", fetched.NumberOfVersions)
	}
}

func TestIntegrationGetServiceByIDNotFound(t *testing.T) {
	db := setupIntegrationDB(t)
	_, err := db.GetServiceByID(context.Background(), 99999)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestIntegrationSearchServices(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "dave@example.com", "dave")
	for i := 1; i <= 3; i++ {
		_, err := db.CreateService(ctx, model.CreateServiceRequest{
			Name:        fmt.Sprintf("Service %d", i),
			Description: "description",
			Version:     model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
			CreatedBy:   owner.ID,
		})
		if err != nil {
			t.Fatalf("seed service %d: %v", i, err)
		}
	}

	t.Run("by name", func(t *testing.T) {
		results, total, err := db.SearchServices(ctx, model.ServiceSearchParams{Name: "Service 2"})
		if err != nil {
			t.Fatalf("SearchServices: %v", err)
		}
		if len(results) != 1 || results[0].Name != "Service 2" {
			t.Fatalf("unexpected results: %+v", results)
		}
		if total != 1 {
			t.Fatalf("expected total 1, got %d", total)
		}
	})

	t.Run("no filter returns all", func(t *testing.T) {
		results, total, err := db.SearchServices(ctx, model.ServiceSearchParams{})
		if err != nil {
			t.Fatalf("SearchServices: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		if total != 3 {
			t.Fatalf("expected total 3, got %d", total)
		}
	})

	t.Run("no match returns empty", func(t *testing.T) {
		results, total, err := db.SearchServices(ctx, model.ServiceSearchParams{Name: "does-not-exist"})
		if err != nil {
			t.Fatalf("SearchServices: %v", err)
		}
		if len(results) != 0 {
			t.Fatalf("expected 0 results, got %d", len(results))
		}
		if total != 0 {
			t.Fatalf("expected total 0, got %d", total)
		}
	})
}

func TestIntegrationCreateAndGetServiceVersions(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "eve@example.com", "eve")
	svc, _ := db.CreateService(ctx, model.CreateServiceRequest{
		Name: "Versioned Service", Description: "desc",
		Version:   model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
		CreatedBy: owner.ID,
	})

	v2, err := db.CreateServiceVersion(ctx, svc.ID, model.CreateVersionRequest{
		VersionString: "v2.0.0", Status: "beta", CreatedBy: owner.ID,
	})
	if err != nil {
		t.Fatalf("CreateServiceVersion: %v", err)
	}
	if v2.VersionString != "v2.0.0" || v2.ServiceID != svc.ID {
		t.Fatalf("unexpected version: %+v", v2)
	}

	versions, err := db.GetServiceVersions(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetServiceVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 versions, got %d", len(versions))
	}
}

func TestIntegrationUpdateService(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "frank@example.com", "frank")
	svc, _ := db.CreateService(ctx, model.CreateServiceRequest{
		Name: "Old Name", Description: "old desc",
		Version:   model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
		CreatedBy: owner.ID,
	})

	svc.Name = "New Name"
	svc.Description = "new desc"
	svc.UpdatedBy = owner.ID
	if err := db.UpdateService(ctx, svc); err != nil {
		t.Fatalf("UpdateService: %v", err)
	}

	fetched, err := db.GetServiceByID(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetServiceByID after update: %v", err)
	}
	if fetched.Name != "New Name" || fetched.Description != "new desc" {
		t.Fatalf("unexpected updated service: %+v", fetched)
	}
}

func TestIntegrationDeleteServiceByID(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "grace@example.com", "grace")
	svc, _ := db.CreateService(ctx, model.CreateServiceRequest{
		Name: "To Delete", Description: "desc",
		Version:   model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
		CreatedBy: owner.ID,
	})

	if err := db.DeleteServiceByID(ctx, svc.ID); err != nil {
		t.Fatalf("DeleteServiceByID: %v", err)
	}

	_, err := db.GetServiceByID(ctx, svc.ID)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows after delete, got %v", err)
	}
}

func TestIntegrationDeleteServiceCascadesVersions(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	owner := seedUser(t, db, "henry@example.com", "henry")
	svc, _ := db.CreateService(ctx, model.CreateServiceRequest{
		Name: "Cascade Service", Description: "desc",
		Version:   model.CreateVersionRequest{VersionString: "v1.0.0", Status: "stable"},
		CreatedBy: owner.ID,
	})
	db.CreateServiceVersion(ctx, svc.ID, model.CreateVersionRequest{
		VersionString: "v2.0.0", Status: "beta", CreatedBy: owner.ID,
	})

	if err := db.DeleteServiceByID(ctx, svc.ID); err != nil {
		t.Fatalf("DeleteServiceByID: %v", err)
	}

	versions, err := db.GetServiceVersions(ctx, svc.ID)
	if err != nil {
		t.Fatalf("GetServiceVersions after delete: %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("expected versions to be cascade-deleted, got %d", len(versions))
	}
}

func TestIntegrationDeleteNonExistentServiceReturnsNotFound(t *testing.T) {
	db := setupIntegrationDB(t)
	err := db.DeleteServiceByID(context.Background(), 99999)
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
