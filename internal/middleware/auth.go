package middleware

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/souvikmndl/search-service/internal/model"
	"go.uber.org/zap"
)

const (
	defaultAuthContextKey = "user"
)

// AuthConfig contains authentication middleware settings.
type AuthConfig struct {
	JWTSecret  string
	UserStore  UserStore
	ContextKey string
	// CookieName is optional. When set, the middleware also accepts a JWT
	// stored in this cookie as a fallback when no Authorization header is present.
	CookieName string
}

// UserStore is the user dependency required by auth middleware.
type UserStore interface {
	GetUserByID(ctx context.Context, id int64) (*model.User, error)
}

// Claims contains the authenticated user identity.
type Claims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// RequireAuthWithConfig returns middleware that authenticates bearer JWTs and verifies the user still exists.
func (config *AuthConfig) RequireAuthWithConfig() echo.MiddlewareFunc {
	if config == nil {
		return missingAuthConfigMiddleware
	}
	if config.JWTSecret == "" {
		return missingJWTSecretMiddleware
	}
	if config.UserStore == nil {
		return missingUserStoreMiddleware
	}

	contextKey := config.ContextKey
	if contextKey == "" {
		contextKey = defaultAuthContextKey
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tokenString, err := extractBearerToken(c.Request().Header.Get(echo.HeaderAuthorization))
			if err != nil && config.CookieName != "" {
				if cookie, cookieErr := c.Cookie(config.CookieName); cookieErr == nil && cookie.Value != "" {
					tokenString = cookie.Value
					err = nil
				}
			}
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing or malformed bearer token")
			}

			claims, err := parseClaims(tokenString, config.JWTSecret)
			if err != nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
			}

			if claims.UserID == 0 || claims.Email == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "token is missing required claims")
			}

			user, err := config.UserStore.GetUserByID(c.Request().Context(), claims.UserID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return echo.NewHTTPError(http.StatusUnauthorized, "token user does not exist")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "could not verify token user")
			}

			if user.EmailID != claims.Email {
				return echo.NewHTTPError(http.StatusUnauthorized, "token email does not match user")
			}

			c.Set(contextKey, claims)
			return next(c)
		}
	}
}

func missingAuthConfigMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "auth middleware is not configured")
	}
}

func missingJWTSecretMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "auth middleware is not configured")
	}
}

func missingUserStoreMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		return echo.NewHTTPError(http.StatusInternalServerError, "auth user verifier is not configured")
	}
}

func extractBearerToken(authorizationHeader string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorizationHeader, prefix) {
		return "", errors.New("missing bearer prefix")
	}

	token := strings.TrimSpace(strings.TrimPrefix(authorizationHeader, prefix))
	if token == "" {
		return "", errors.New("missing bearer token")
	}

	return token, nil
}

func parseClaims(tokenString string, jwtSecret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

func Latency(logger *zap.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			logger.Info("request",
				zap.String("method", c.Request().Method),
				zap.String("path", c.Request().URL.Path),
				zap.Int("status", c.Response().Status),
				zap.Duration("latency", time.Since(start)),
				zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
			)
			return err
		}
	}
}
