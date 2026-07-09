package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/souvikmndl/search-service/internal/model"
)

const testJWTSecret = "test-secret"

type mockUserStore struct {
	user        *model.User
	err         error
	requestedID int64
	calls       int
}

func (m *mockUserStore) GetUserByID(ctx context.Context, id int64) (*model.User, error) {
	m.calls++
	m.requestedID = id
	if m.err != nil {
		return nil, m.err
	}
	return m.user, nil
}

func TestRequireAuthWithConfigAcceptsValidBearerTokenAndVerifiedUser(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if store.calls != 1 {
		t.Fatalf("expected user store to be called once, got %d", store.calls)
	}
	if store.requestedID != 42 {
		t.Fatalf("expected lookup id %d, got %d", 42, store.requestedID)
	}
}

func TestRequireAuthWithConfigStoresClaimsInContext(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := echo.New()
	e.GET("/protected", func(c echo.Context) error {
		claims, ok := c.Get("auth_user").(*Claims)
		if !ok {
			t.Fatalf("expected claims in context")
		}
		if claims.UserID != 42 || claims.Email != "user@example.com" {
			t.Fatalf("unexpected claims: %+v", claims)
		}
		return c.NoContent(http.StatusOK)
	}, (&AuthConfig{
		JWTSecret:  testJWTSecret,
		UserStore:  store,
		ContextKey: "auth_user",
	}).RequireAuthWithConfig())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireAuthWithConfigRejectsMissingBearerToken(t *testing.T) {
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthWithConfigRejectsInvalidToken(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer invalid-token")
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if store.calls != 0 {
		t.Fatalf("expected user store not to be called, got %d calls", store.calls)
	}
}

func TestRequireAuthWithConfigRejectsMissingRequiredClaims(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTokenWithClaims(t, testJWTSecret, jwt.MapClaims{
		"exp": time.Now().Add(time.Hour).Unix(),
	}))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if store.calls != 0 {
		t.Fatalf("expected user store not to be called, got %d calls", store.calls)
	}
}

func TestRequireAuthWithConfigRejectsMissingUser(t *testing.T) {
	store := &mockUserStore{err: sql.ErrNoRows}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthWithConfigRejectsEmailMismatch(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "other@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthWithConfigReturnsServerErrorForStoreFailure(t *testing.T) {
	store := &mockUserStore{err: errors.New("db unavailable")}
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRequireAuthWithConfigReturnsServerErrorWhenSecretMissing(t *testing.T) {
	e := newTestEcho(AuthConfig{
		UserStore: &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRequireAuthWithConfigReturnsServerErrorWhenConfigMissing(t *testing.T) {
	e := echo.New()
	var config *AuthConfig
	e.GET("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, config.RequireAuthWithConfig())

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRequireAuthWithConfigReturnsServerErrorWhenUserStoreMissing(t *testing.T) {
	e := newTestEcho(AuthConfig{JWTSecret: testJWTSecret})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestRequireAuthWithConfigAcceptsValidCookieToken(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret:  testJWTSecret,
		UserStore:  store,
		CookieName: "session",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: signedTestToken(t, testJWTSecret, 42, "user@example.com"),
	})
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d for cookie auth, got %d", http.StatusOK, rec.Code)
	}
	if store.calls != 1 {
		t.Fatalf("expected user store called once, got %d", store.calls)
	}
}

func TestRequireAuthWithConfigHeaderTakesPrecedenceOverCookie(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{
		JWTSecret:  testJWTSecret,
		UserStore:  store,
		CookieName: "session",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	// Valid header token — should be used; cookie with wrong secret is ignored.
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	req.AddCookie(&http.Cookie{
		Name:  "session",
		Value: signedTestToken(t, "wrong-secret", 99, "other@example.com"),
	})
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestRequireAuthWithConfigRejectsWhenNeitherHeaderNorCookiePresent(t *testing.T) {
	e := newTestEcho(AuthConfig{
		JWTSecret:  testJWTSecret,
		UserStore:  &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}},
		CookieName: "session",
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestRequireAuthWithConfigRejectsExpiredToken(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{JWTSecret: testJWTSecret, UserStore: store})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTokenWithClaims(t, testJWTSecret, jwt.MapClaims{
		"user_id": float64(42),
		"email":   "user@example.com",
		"exp":     time.Now().Add(-time.Hour).Unix(),
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
	}))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for expired token, got %d", http.StatusUnauthorized, rec.Code)
	}
	if store.calls != 0 {
		t.Fatalf("user store must not be called for an expired token, got %d calls", store.calls)
	}
}

func TestRequireAuthWithConfigRejectsTokenSignedWithWrongSecret(t *testing.T) {
	store := &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}}
	e := newTestEcho(AuthConfig{JWTSecret: testJWTSecret, UserStore: store})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+signedTestToken(t, "completely-different-secret", 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for wrong-secret token, got %d", http.StatusUnauthorized, rec.Code)
	}
	if store.calls != 0 {
		t.Fatalf("user store must not be called when signature is invalid, got %d calls", store.calls)
	}
}

func TestRequireAuthWithConfigRejectsLowercaseBearerPrefix(t *testing.T) {
	e := newTestEcho(AuthConfig{
		JWTSecret: testJWTSecret,
		UserStore: &mockUserStore{user: &model.User{ID: 42, EmailID: "user@example.com"}},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set(echo.HeaderAuthorization, "bearer "+signedTestToken(t, testJWTSecret, 42, "user@example.com"))
	rec := httptest.NewRecorder()

	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for lowercase bearer prefix, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func newTestEcho(config AuthConfig) *echo.Echo {
	e := echo.New()
	e.GET("/protected", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	}, config.RequireAuthWithConfig())
	return e
}

func signedTestToken(t *testing.T, secret string, userID int64, email string) string {
	t.Helper()

	return signedTokenWithClaims(t, secret, jwt.MapClaims{
		"user_id": userID,
		"email":   email,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
	})
}

func signedTokenWithClaims(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign test token: %v", err)
	}

	return signed
}
