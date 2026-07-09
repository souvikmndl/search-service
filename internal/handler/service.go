package handler

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	"github.com/souvikmndl/search-service/internal/config"
	"github.com/souvikmndl/search-service/internal/datastores/postgres"
	"github.com/souvikmndl/search-service/internal/logger"
	authmw "github.com/souvikmndl/search-service/internal/middleware"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// ServiceHandler for search service
type ServiceHandler struct {
	SearchDB  postgres.SearchDB
	Logger    *zap.Logger
	JWTSecret string
}

// RegisterRoutes connects the routes to the handler funcs
func (h *ServiceHandler) RegisterRoutes(e *echo.Echo) {
	e.Use(echomw.RequestID())
	e.Use(echomw.RateLimiterWithConfig(createRateLimiterConfig()))

	e.POST("/signup", h.SignUp)
	e.POST("/login", h.Login)
	e.POST("/logout", h.Logout)

	authConfig := &authmw.AuthConfig{
		JWTSecret:  h.JWTSecret,
		UserStore:  h.SearchDB,
		CookieName: sessionCookieName,
	}
	services := e.Group("/services", authConfig.RequireAuthWithConfig())
	services.POST("", h.CreateService)
	services.GET("", h.Search)
	services.GET("/:id", h.GetServiceByID)
	services.POST("/:id/versions", h.CreateServiceVersion)
	services.GET("/:id/versions", h.GetServiceVersions)
	services.PUT("/:id", h.UpdateServiceByID)
	services.DELETE("/:id", h.DeleteServiceByID)
}

// NewServiceHandler is constructor for ServiceHandler
func NewServiceHandler(searchDB postgres.SearchDB, logger *zap.Logger, jwtSecret string) *ServiceHandler {
	return &ServiceHandler{
		SearchDB:  searchDB,
		Logger:    logger,
		JWTSecret: jwtSecret,
	}
}

// InitService starts the services necessary
func InitService(config *config.Config) (*echo.Echo, *sql.DB) {
	sqlDB, err := postgres.NewDBConnection(config)
	if err != nil {
		log.Fatalf("error connecting to db: %v", err)
	}
	dbConn := postgres.NewDB(sqlDB)

	logFile := fmt.Sprintf("%s%s", config.Log.LogDir, config.Log.LogFile)
	logger := logger.NewLogger(config.Log.Level, logFile)

	if config.Auth.JWTSecret == "" {
		log.Fatal("JWT_SECRET must be set")
	}

	serviceHandler := NewServiceHandler(dbConn, logger, config.Auth.JWTSecret)
	e := echo.New()
	serviceHandler.RegisterRoutes(e)

	return e, sqlDB
}

func createRateLimiterConfig() echomw.RateLimiterConfig {
	// TODO: make rate limit values config driven
	storeConfig := echomw.RateLimiterMemoryStoreConfig{
		Rate:      rate.Limit(20),
		Burst:     100,
		ExpiresIn: 3 * time.Minute,
	}

	return echomw.RateLimiterConfig{
		Skipper: echomw.DefaultSkipper,
		Store:   echomw.NewRateLimiterMemoryStoreWithConfig(storeConfig),
		IdentifierExtractor: func(ctx echo.Context) (string, error) {
			return ctx.RealIP(), nil
		},
		DenyHandler: func(context echo.Context, identifier string, err error) error {
			return context.JSON(http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
		},
	}
}
