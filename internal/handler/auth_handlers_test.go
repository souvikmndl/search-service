package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/souvikmndl/search-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func TestSignUpCreatesUser(t *testing.T) {
	var captured model.User
	db := &mockSearchDB{
		createUserFunc: func(ctx context.Context, user *model.User) error {
			captured = *user
			user.ID = 10
			user.CreatedAt = time.Now()
			return nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/signup", `{"email_id":" user@example.com ","user_name":" tester ","password":"secret1"}`)

	err := h.SignUp(c)

	if err != nil {
		t.Fatalf("signup returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)
	if captured.EmailID != "user@example.com" || captured.UserName != "tester" {
		t.Fatalf("unexpected user payload: %+v", captured)
	}
	if bcrypt.CompareHashAndPassword([]byte(captured.Password), []byte("secret1")) != nil {
		t.Fatalf("expected password to be bcrypt hashed")
	}
	if strings.Contains(rec.Body.String(), "secret1") || strings.Contains(rec.Body.String(), captured.Password) {
		t.Fatalf("response leaked password: %s", rec.Body.String())
	}
}

func TestSignUpRejectsBadPayloads(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed json", body: `{`},
		{name: "missing email", body: `{"user_name":"tester","password":"secret1"}`},
		{name: "whitespace only email", body: `{"email_id":"   ","user_name":"tester","password":"secret1"}`},
		{name: "missing username", body: `{"email_id":"user@example.com","password":"secret1"}`},
		{name: "short password", body: `{"email_id":"user@example.com","user_name":"tester","password":"12345"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(&mockSearchDB{})
			c, rec := newTestContext(http.MethodPost, "/signup", tt.body)

			if err := h.SignUp(c); err != nil {
				t.Fatalf("signup returned error: %v", err)
			}
			assertStatus(t, rec, http.StatusBadRequest)
		})
	}
}

func TestSignUpHandlesDuplicateAndServerErrors(t *testing.T) {
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
				createUserFunc: func(ctx context.Context, user *model.User) error {
					return tt.err
				},
			}
			h := newTestHandler(db)
			c, rec := newTestContext(http.MethodPost, "/signup", `{"email_id":"user@example.com","user_name":"tester","password":"secret1"}`)

			if err := h.SignUp(c); err != nil {
				t.Fatalf("signup returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestLoginSetsSessionCookie(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("secret1"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	db := &mockSearchDB{
		getUserByEmailFunc: func(ctx context.Context, emailID string) (*model.User, error) {
			if emailID != "user@example.com" {
				t.Fatalf("unexpected email lookup: %s", emailID)
			}
			return &model.User{ID: 11, EmailID: emailID, UserName: "tester", Password: string(hashed), CreatedAt: time.Now()}, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/login", `{"email_id":" user@example.com ","password":"secret1"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)
	if len(rec.Result().Cookies()) == 0 || rec.Result().Cookies()[0].Name != sessionCookieName {
		t.Fatalf("expected session cookie, got %+v", rec.Result().Cookies())
	}
}

func TestLoginRejectsBadInputsAndCredentials(t *testing.T) {
	hashed, err := bcrypt.GenerateFromPassword([]byte("secret1"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	tests := []struct {
		name string
		body string
		db   *mockSearchDB
		want int
	}{
		{name: "malformed json", body: `{`, db: &mockSearchDB{}, want: http.StatusBadRequest},
		{name: "missing password", body: `{"email_id":"user@example.com"}`, db: &mockSearchDB{}, want: http.StatusBadRequest},
		{name: "whitespace only email", body: `{"email_id":"   ","password":"secret1"}`, db: &mockSearchDB{}, want: http.StatusBadRequest},
		{
			name: "unknown user",
			body: `{"email_id":"user@example.com","password":"secret1"}`,
			db: &mockSearchDB{getUserByEmailFunc: func(ctx context.Context, emailID string) (*model.User, error) {
				return nil, sql.ErrNoRows
			}},
			want: http.StatusUnauthorized,
		},
		{
			name: "db error",
			body: `{"email_id":"user@example.com","password":"secret1"}`,
			db: &mockSearchDB{getUserByEmailFunc: func(ctx context.Context, emailID string) (*model.User, error) {
				return nil, errors.New("db down")
			}},
			want: http.StatusInternalServerError,
		},
		{
			name: "wrong password",
			body: `{"email_id":"user@example.com","password":"wrong"}`,
			db: &mockSearchDB{getUserByEmailFunc: func(ctx context.Context, emailID string) (*model.User, error) {
				return &model.User{ID: 11, EmailID: emailID, Password: string(hashed)}, nil
			}},
			want: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(tt.db)
			c, rec := newTestContext(http.MethodPost, "/login", tt.body)

			if err := h.Login(c); err != nil {
				t.Fatalf("login returned error: %v", err)
			}
			assertStatus(t, rec, tt.want)
		})
	}
}

func TestSignUpValidationErrorsAreStructured(t *testing.T) {
	h := newTestHandler(&mockSearchDB{})
	// All three fields fail: empty email after trim, empty username, password too short.
	c, rec := newTestContext(http.MethodPost, "/signup", `{"email_id":"  ","user_name":"","password":"abc"}`)

	if err := h.SignUp(c); err != nil {
		t.Fatalf("SignUp returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusBadRequest)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["message"] != "validation failed" {
		t.Fatalf("expected message 'validation failed', got %v", body["message"])
	}
	errs, ok := body["errors"].([]any)
	if !ok || len(errs) != 3 {
		t.Fatalf("expected 3 validation errors, got %v", body["errors"])
	}
	fields := make(map[string]string, 3)
	for _, e := range errs {
		m := e.(map[string]any)
		fields[m["field"].(string)] = m["message"].(string)
	}
	for _, f := range []string{"email_id", "user_name", "password"} {
		if fields[f] == "" {
			t.Errorf("expected validation error for field %q", f)
		}
	}
}

func TestSignUpResponseBodyContainsUserData(t *testing.T) {
	db := &mockSearchDB{
		createUserFunc: func(ctx context.Context, user *model.User) error {
			user.ID = 10
			user.CreatedAt = time.Now()
			return nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/signup", `{"email_id":"user@example.com","user_name":"tester","password":"secret1"}`)

	if err := h.SignUp(c); err != nil {
		t.Fatalf("signup returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusCreated)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got %v", body)
	}
	if data["id"] == nil {
		t.Fatalf("expected id in response data")
	}
	if data["email_id"] != "user@example.com" {
		t.Fatalf("expected email_id in response data, got %v", data["email_id"])
	}
	if _, hasPassword := data["password"]; hasPassword {
		t.Fatalf("response must not expose password field")
	}
}

func TestLoginResponseBodyContainsUserData(t *testing.T) {
	hashed, _ := bcrypt.GenerateFromPassword([]byte("secret1"), bcrypt.DefaultCost)
	db := &mockSearchDB{
		getUserByEmailFunc: func(ctx context.Context, emailID string) (*model.User, error) {
			return &model.User{ID: 11, EmailID: emailID, UserName: "tester", Password: string(hashed), CreatedAt: time.Now()}, nil
		},
	}
	h := newTestHandler(db)
	c, rec := newTestContext(http.MethodPost, "/login", `{"email_id":"user@example.com","password":"secret1"}`)

	if err := h.Login(c); err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in response, got %v", body)
	}
	if data["email_id"] != "user@example.com" {
		t.Fatalf("expected email_id in response, got %v", data["email_id"])
	}
	if _, hasPassword := data["password"]; hasPassword {
		t.Fatalf("login response must not expose password field")
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	h := newTestHandler(&mockSearchDB{})
	c, rec := newTestContext(http.MethodPost, "/logout", "")

	if err := h.Logout(c); err != nil {
		t.Fatalf("logout returned error: %v", err)
	}
	assertStatus(t, rec, http.StatusOK)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || cookies[0].MaxAge != -1 {
		t.Fatalf("expected expired session cookie, got %+v", cookies)
	}
}
