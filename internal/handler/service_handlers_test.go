package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/souvikmndl/search-service/internal/model"
)

func TestCreateServiceSetsCreatedBy(t *testing.T) {
	var captured model.CreateServiceRequest
	db := &mockSearchDB{
		createServiceFunc: func(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error) {
			captured = req
			svc := testService(10)
			svc.CreatedBy = req.CreatedBy
			svc.UpdatedBy = req.CreatedBy
			return svc, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/services", `{"name":"svc","description":"desc","version":{"version_string":"v1.0.0","status":"stable"}}`)
	setAuthClaims(c, 7, "user@example.com")

	if err := h.CreateService(c); err != nil {
		t.Fatalf("create service returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
	if captured.CreatedBy != 7 {
		t.Fatalf("expected created_by 7, got %d", captured.CreatedBy)
	}
}

func TestCreateServiceRejectsInvalidPayloadAndMissingClaims(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		setClaims bool
	}{
		{name: "malformed json", body: `{`, setClaims: true},
		{name: "missing name", body: `{"description":"desc","version":{"version_string":"v1","status":"stable"}}`, setClaims: true},
		{name: "missing version", body: `{"name":"svc","description":"desc"}`, setClaims: true},
		{name: "missing claims", body: `{"name":"svc","description":"desc","version":{"version_string":"v1","status":"stable"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(&mockSearchDB{})
			c, rec := newTestContext(http.MethodPost, "/services", tt.body)
			if tt.setClaims {
				setAuthClaims(c, 7, "user@example.com")
			}

			if err := h.CreateService(c); err != nil {
				t.Fatalf("create service returned error: %v", err)
			}
			if tt.name == "missing claims" {
				assertStatus(t, rec, http.StatusInternalServerError)
				return
			}
			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}

func TestCreateServiceHandlesDuplicateAndServerErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "duplicate", err: errors.New("duplicate key value violates unique constraint"), want: http.StatusBadRequest},
		{name: "db error", err: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				createServiceFunc: func(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error) {
					return nil, tt.err
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodPost, "/services", `{"name":"svc","description":"desc","version":{"version_string":"v1","status":"stable"}}`)
			setAuthClaims(c, 7, "user@example.com")

			if err := h.CreateService(c); err != nil {
				t.Fatalf("create service returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestSearchServices(t *testing.T) {
	db := &mockSearchDB{
		searchServicesFunc: func(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error) {
			if params.Name != "svc" {
				t.Fatalf("expected query param name svc, got %q", params.Name)
			}
			return []model.Service{*testService(10)}, 1, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services?name=svc", "")

	if err := h.Search(c); err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestSearchServicesForwardsAllParams(t *testing.T) {
	var captured model.ServiceSearchParams
	db := &mockSearchDB{
		searchServicesFunc: func(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error) {
			captured = params
			return []model.Service{*testService(10)}, 1, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services?name=api&description=gateway&page=2&page_size=20&sort_by=name&order=desc", "")

	if err := h.Search(c); err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)

	if captured.Name != "api" {
		t.Errorf("Name: expected api, got %q", captured.Name)
	}
	if captured.Description != "gateway" {
		t.Errorf("Description: expected gateway, got %q", captured.Description)
	}
	if captured.Page != 2 {
		t.Errorf("Page: expected 2, got %d", captured.Page)
	}
	if captured.PageSize != 20 {
		t.Errorf("PageSize: expected 20, got %d", captured.PageSize)
	}
	if captured.SortBy != "name" {
		t.Errorf("SortBy: expected name, got %q", captured.SortBy)
	}
	if captured.Order != "desc" {
		t.Errorf("Order: expected desc, got %q", captured.Order)
	}
}

func TestSearchServicesReturnsEmptySliceNotNull(t *testing.T) {
	db := &mockSearchDB{
		searchServicesFunc: func(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error) {
			return nil, 0, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services?name=nonexistent", "")

	if err := h.Search(c); err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)

	body := rec.Body.String()
	// The response data must be an array (even if empty), not JSON null.
	if !strings.Contains(body, `"data":[]`) {
		t.Fatalf("expected empty array in response, got: %s", body)
	}
}

func TestSearchServicesHandlesDBError(t *testing.T) {
	db := &mockSearchDB{
		searchServicesFunc: func(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error) {
			return nil, 0, errors.New("db down")
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services", "")

	if err := h.Search(c); err != nil {
		t.Fatalf("search returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusInternalServerError)
}

func TestGetServiceByID(t *testing.T) {
	db := &mockSearchDB{
		getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
			if id != 10 {
				t.Fatalf("expected id 10, got %d", id)
			}
			return testService(id), nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services/10", "")
	setPath(c, "/services/:id", []string{"id"}, []string{"10"})

	if err := h.GetServiceByID(c); err != nil {
		t.Fatalf("get service returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestGetServiceByIDHandlesInvalidNotFoundAndServerErrors(t *testing.T) {
	tests := []struct {
		name string
		id   string
		err  error
		want int
	}{
		{name: "invalid id", id: "abc", want: http.StatusBadRequest},
		{name: "not found", id: "10", err: sql.ErrNoRows, want: http.StatusNotFound},
		{name: "db error", id: "10", err: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
					return nil, tt.err
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodGet, "/services/"+tt.id, "")
			setPath(c, "/services/:id", []string{"id"}, []string{tt.id})

			if err := h.GetServiceByID(c); err != nil {
				t.Fatalf("get service returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestCreateServiceVersionSetsCreatedBy(t *testing.T) {
	var captured model.CreateVersionRequest
	db := &mockSearchDB{
		createServiceVersionFunc: func(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error) {
			if serviceID != 10 {
				t.Fatalf("expected service id 10, got %d", serviceID)
			}
			captured = version
			v := testVersion(2)
			v.CreatedBy = version.CreatedBy
			return v, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/services/10/versions", `{"version_string":"v2.0.0","status":"stable"}`)
	setPath(c, "/services/:id/versions", []string{"id"}, []string{"10"})
	setAuthClaims(c, 7, "user@example.com")

	if err := h.CreateServiceVersion(c); err != nil {
		t.Fatalf("create version returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
	if captured.CreatedBy != 7 {
		t.Fatalf("expected created_by 7, got %d", captured.CreatedBy)
	}
}

func TestCreateServiceVersionHandlesEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		body      string
		setClaims bool
		dbErr     error
		want      int
	}{
		{name: "invalid id", id: "abc", body: `{}`, setClaims: true, want: http.StatusBadRequest},
		{name: "malformed json", id: "10", body: `{`, setClaims: true, want: http.StatusBadRequest},
		{name: "missing status", id: "10", body: `{"version_string":"v2"}`, setClaims: true, want: http.StatusBadRequest},
		{name: "missing claims", id: "10", body: `{"version_string":"v2","status":"stable"}`, want: http.StatusInternalServerError},
		{name: "db error", id: "10", body: `{"version_string":"v2","status":"stable"}`, setClaims: true, dbErr: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				createServiceVersionFunc: func(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error) {
					return nil, tt.dbErr
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodPost, "/services/"+tt.id+"/versions", tt.body)
			setPath(c, "/services/:id/versions", []string{"id"}, []string{tt.id})
			if tt.setClaims {
				setAuthClaims(c, 7, "user@example.com")
			}

			if err := h.CreateServiceVersion(c); err != nil {
				t.Fatalf("create version returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestGetServiceVersions(t *testing.T) {
	db := &mockSearchDB{
		getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
			return testService(id), nil
		},
		getServiceVersionsFunc: func(ctx context.Context, serviceID int64) ([]model.Version, error) {
			return []model.Version{*testVersion(2)}, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodGet, "/services/10/versions", "")
	setPath(c, "/services/:id/versions", []string{"id"}, []string{"10"})

	if err := h.GetServiceVersions(c); err != nil {
		t.Fatalf("get versions returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestGetServiceVersionsHandlesErrors(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		serviceErr error
		versionErr error
		want       int
	}{
		{name: "invalid id", id: "abc", want: http.StatusBadRequest},
		{name: "service not found", id: "10", serviceErr: sql.ErrNoRows, want: http.StatusNotFound},
		{name: "service db error", id: "10", serviceErr: errors.New("db down"), want: http.StatusInternalServerError},
		{name: "versions db error", id: "10", versionErr: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
					if tt.serviceErr != nil {
						return nil, tt.serviceErr
					}
					return testService(id), nil
				},
				getServiceVersionsFunc: func(ctx context.Context, serviceID int64) ([]model.Version, error) {
					return nil, tt.versionErr
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodGet, "/services/"+tt.id+"/versions", "")
			setPath(c, "/services/:id/versions", []string{"id"}, []string{tt.id})

			if err := h.GetServiceVersions(c); err != nil {
				t.Fatalf("get versions returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestUpdateServiceByIDSetsUpdatedBy(t *testing.T) {
	var updated *model.Service
	db := &mockSearchDB{
		getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
			return testService(id), nil
		},
		updateServiceFunc: func(ctx context.Context, service *model.Service) error {
			updated = service
			return nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPut, "/services/10", `{"name":"new svc","description":"new desc"}`)
	setPath(c, "/services/:id", []string{"id"}, []string{"10"})
	setAuthClaims(c, 99, "user@example.com")

	if err := h.UpdateServiceByID(c); err != nil {
		t.Fatalf("update service returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
	if updated == nil || updated.UpdatedBy != 99 || updated.Name != "new svc" {
		t.Fatalf("unexpected updated service: %+v", updated)
	}
}

func TestUpdateServiceByIDHandlesErrors(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		body      string
		setClaims bool
		getErr    error
		updateErr error
		want      int
	}{
		{name: "invalid id", id: "abc", body: `{}`, setClaims: true, want: http.StatusBadRequest},
		{name: "malformed json", id: "10", body: `{`, setClaims: true, want: http.StatusBadRequest},
		{name: "short name", id: "10", body: `{"name":"ab"}`, setClaims: true, want: http.StatusBadRequest},
		{name: "not found", id: "10", body: `{"name":"new svc"}`, setClaims: true, getErr: sql.ErrNoRows, want: http.StatusNotFound},
		{name: "get db error", id: "10", body: `{"name":"new svc"}`, setClaims: true, getErr: errors.New("db down"), want: http.StatusInternalServerError},
		{name: "missing claims", id: "10", body: `{"name":"new svc"}`, want: http.StatusInternalServerError},
		{name: "duplicate", id: "10", body: `{"name":"new svc"}`, setClaims: true, updateErr: errors.New("duplicate key value violates unique constraint"), want: http.StatusBadRequest},
		{name: "update db error", id: "10", body: `{"name":"new svc"}`, setClaims: true, updateErr: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				getServiceByIDFunc: func(ctx context.Context, id int64) (*model.Service, error) {
					if tt.getErr != nil {
						return nil, tt.getErr
					}
					return testService(id), nil
				},
				updateServiceFunc: func(ctx context.Context, service *model.Service) error {
					return tt.updateErr
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodPut, "/services/"+tt.id, tt.body)
			setPath(c, "/services/:id", []string{"id"}, []string{tt.id})
			if tt.setClaims {
				setAuthClaims(c, 7, "user@example.com")
			}

			if err := h.UpdateServiceByID(c); err != nil {
				t.Fatalf("update service returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestDeleteServiceByID(t *testing.T) {
	db := &mockSearchDB{
		deleteServiceByIDFunc: func(ctx context.Context, id int64) error {
			if id != 10 {
				t.Fatalf("expected id 10, got %d", id)
			}
			return nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodDelete, "/services/10", "")
	setPath(c, "/services/:id", []string{"id"}, []string{"10"})

	if err := h.DeleteServiceByID(c); err != nil {
		t.Fatalf("delete service returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
}

func TestDeleteServiceByIDHandlesErrors(t *testing.T) {
	tests := []struct {
		name string
		id   string
		err  error
		want int
	}{
		{name: "invalid id", id: "abc", want: http.StatusBadRequest},
		{name: "not found", id: "10", err: sql.ErrNoRows, want: http.StatusNotFound},
		{name: "db error", id: "10", err: errors.New("db down"), want: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockSearchDB{
				deleteServiceByIDFunc: func(ctx context.Context, id int64) error {
					return tt.err
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodDelete, "/services/"+tt.id, "")
			setPath(c, "/services/:id", []string{"id"}, []string{tt.id})

			if err := h.DeleteServiceByID(c); err != nil {
				t.Fatalf("delete service returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}
