package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/souvikmndl/search-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "session"

// SignUp creates a user account.
func (h *ServiceHandler) SignUp(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)
	logger.Info("Sign Up Endpoint")

	var req model.SignUpRequest
	if err := c.Bind(&req); err != nil {
		logger.Errorf("invalid request body %v", err)
		return h.badRequestErrorResponse(c)
	}

	req.EmailID = strings.TrimSpace(req.EmailID)
	req.UserName = strings.TrimSpace(req.UserName)
	var signUpErrs []ValidationError
	if req.EmailID == "" {
		signUpErrs = append(signUpErrs, ValidationError{Field: "email_id", Message: "is required"})
	}
	if req.UserName == "" {
		signUpErrs = append(signUpErrs, ValidationError{Field: "user_name", Message: "is required"})
	}
	if len(req.Password) < 6 {
		signUpErrs = append(signUpErrs, ValidationError{Field: "password", Message: "must be at least 6 characters"})
	}
	if len(signUpErrs) > 0 {
		logger.Error("sign-up validation failed")
		return h.validationErrorResponse(c, signUpErrs)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		logger.Errorf("error hashing password %v", err)
		return h.serverErrorResponse(c)
	}

	user := model.User{
		EmailID:  req.EmailID,
		UserName: req.UserName,
		Password: string(hashedPassword),
	}

	if err := h.SearchDB.CreateUser(c.Request().Context(), &user); err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			logger.Errorf("duplicate user: %v", err)
			return h.errorResponse(c, http.StatusBadRequest, "user with given email exists")
		}
		logger.Errorf("error creating user: %v", err)
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusCreated, envelope{"data": model.TransformToUserResponse(user)})
}

// Login validates credentials and sets a session cookie.
func (h *ServiceHandler) Login(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)
	logger.Info("Login Endpoint")

	var req model.LoginRequest
	if err := c.Bind(&req); err != nil {
		logger.Errorf("invalid request body %v", err)
		return h.badRequestErrorResponse(c)
	}

	req.EmailID = strings.TrimSpace(req.EmailID)
	var loginErrs []ValidationError
	if req.EmailID == "" {
		loginErrs = append(loginErrs, ValidationError{Field: "email_id", Message: "is required"})
	}
	if req.Password == "" {
		loginErrs = append(loginErrs, ValidationError{Field: "password", Message: "is required"})
	}
	if len(loginErrs) > 0 {
		logger.Error("login validation failed")
		return h.validationErrorResponse(c, loginErrs)
	}

	user, err := h.SearchDB.GetUserByEmail(c.Request().Context(), req.EmailID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("user with given email doesnot exits %s %v", req.EmailID, err)
			return h.errorResponse(c, http.StatusUnauthorized, "invalid email or password")
		}
		logger.Errorf("unable to get email from db, error: %v", err)
		return h.serverErrorResponse(c)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		logger.Error("invalid email or password, compare and hash failed")
		return h.errorResponse(c, http.StatusUnauthorized, "invalid email or password")
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	token, err := h.createAuthToken(user, expiresAt)
	if err != nil {
		logger.Errorf("error creating auth token, error: %v", err)
		return h.serverErrorResponse(c)
	}

	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return c.JSON(http.StatusOK, envelope{"data": model.TransformToUserResponse(*user)})
}

// Logout clears the session cookie.
func (h *ServiceHandler) Logout(c echo.Context) error {
	c.SetCookie(&http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return c.JSON(http.StatusOK, envelope{"message": "logged out"})
}

func (h *ServiceHandler) createAuthToken(user *model.User, expiresAt time.Time) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.EmailID,
		"exp":     expiresAt.Unix(),
		"iat":     time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.JWTSecret))
}
