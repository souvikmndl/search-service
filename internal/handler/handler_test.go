package handler

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	authmw "github.com/souvikmndl/search-service/internal/middleware"
	"github.com/souvikmndl/search-service/internal/model"
	"go.uber.org/zap"
)

const testJWTSecret = "test-secret"

type mockSearchDB struct {
	createUserFunc           func(ctx context.Context, user *model.User) error
	getUserByIDFunc          func(ctx context.Context, id int64) (*model.User, error)
	getUserByEmailFunc       func(ctx context.Context, emailID string) (*model.User, error)
	searchServicesFunc       func(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error)
	createServiceFunc        func(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error)
	getServiceByIDFunc       func(ctx context.Context, id int64) (*model.Service, error)
	createServiceVersionFunc func(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error)
	getServiceVersionsFunc   func(ctx context.Context, serviceID int64) ([]model.Version, error)
	updateServiceFunc        func(ctx context.Context, service *model.Service) error
	deleteServiceByIDFunc    func(ctx context.Context, id int64) error
}

func (m *mockSearchDB) CreateUser(ctx context.Context, user *model.User) error {
	return m.createUserFunc(ctx, user)
}

func (m *mockSearchDB) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	return m.getUserByIDFunc(ctx, id)
}

func (m *mockSearchDB) GetUserByEmail(ctx context.Context, emailID string) (*model.User, error) {
	return m.getUserByEmailFunc(ctx, emailID)
}

func (m *mockSearchDB) SearchServices(ctx context.Context, params model.ServiceSearchParams) ([]model.Service, int64, error) {
	return m.searchServicesFunc(ctx, params)
}

func (m *mockSearchDB) CreateService(ctx context.Context, req model.CreateServiceRequest) (*model.Service, error) {
	return m.createServiceFunc(ctx, req)
}

func (m *mockSearchDB) GetServiceByID(ctx context.Context, id int64) (*model.Service, error) {
	return m.getServiceByIDFunc(ctx, id)
}

func (m *mockSearchDB) CreateServiceVersion(ctx context.Context, serviceID int64, version model.CreateVersionRequest) (*model.Version, error) {
	return m.createServiceVersionFunc(ctx, serviceID, version)
}

func (m *mockSearchDB) GetServiceVersions(ctx context.Context, serviceID int64) ([]model.Version, error) {
	return m.getServiceVersionsFunc(ctx, serviceID)
}

func (m *mockSearchDB) UpdateService(ctx context.Context, service *model.Service) error {
	return m.updateServiceFunc(ctx, service)
}

func (m *mockSearchDB) DeleteServiceByID(ctx context.Context, id int64) error {
	return m.deleteServiceByIDFunc(ctx, id)
}

func newTestHandler(db *mockSearchDB) *ServiceHandler {
	return NewServiceHandler(db, zap.NewNop(), testJWTSecret)
}

func newTestContext(method string, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func setPath(c echo.Context, path string, names []string, values []string) {
	c.SetPath(path)
	c.SetParamNames(names...)
	c.SetParamValues(values...)
}

func setAuthClaims(c echo.Context, userID int64, email string) {
	c.Set("user", &authmw.Claims{UserID: userID, Email: email})
}

func testService(id int64) *model.Service {
	now := time.Now()
	return &model.Service{
		ID:               id,
		Name:             "test service",
		Description:      "description",
		NumberOfVersions: 1,
		CreatedBy:        7,
		UpdatedBy:        7,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func testVersion(id int64) *model.Version {
	return &model.Version{
		ID:            id,
		ServiceID:     10,
		VersionString: "v1.0.0",
		Status:        "stable",
		CreatedBy:     7,
		CreatedAt:     time.Now(),
	}
}

func assertStatus(t testingT, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("expected status %d, got %d body=%s", want, rec.Code, rec.Body.String())
	}
}

type testingT interface {
	Helper()
	Fatalf(format string, args ...any)
}
