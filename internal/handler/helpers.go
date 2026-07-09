package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type envelope map[string]any

// ValidationError describes a single field-level validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (h *ServiceHandler) validationErrorResponse(c echo.Context, errs []ValidationError) error {
	return c.JSON(http.StatusBadRequest, envelope{
		"message": "validation failed",
		"errors":  errs,
	})
}

func (h *ServiceHandler) errorResponse(c echo.Context, status int, message string) error {
	return c.JSON(status, envelope{"message": message})
}

func (h *ServiceHandler) serverErrorResponse(c echo.Context) error {
	message := "the server encountered a problem and could not process your request"
	return h.errorResponse(c, http.StatusInternalServerError, message)
}

func (h *ServiceHandler) badRequestErrorResponse(c echo.Context) error {
	message := "the request payload is invalid"
	return h.errorResponse(c, http.StatusBadRequest, message)
}

func (h *ServiceHandler) invalidIDErrResponse(c echo.Context) error {
	message := "the resource id is invalid"
	return h.errorResponse(c, http.StatusBadRequest, message)
}

func (h *ServiceHandler) notFoundErrResponse(c echo.Context) error {
	message := "resource with given id does not exist"
	return h.errorResponse(c, http.StatusNotFound, message)
}

func (h *ServiceHandler) duplicateTitleErrResponse(c echo.Context) error {
	message := "service with given title exists"
	return h.errorResponse(c, http.StatusBadRequest, message)
}

// LoggerWithFields adds request_id and request_uri to logs
func LoggerWithFields(logger *zap.Logger, c echo.Context) *zap.SugaredLogger {
	return logger.With(
		zap.String("request_id", c.Response().Header().Get(echo.HeaderXRequestID)),
		zap.String("request_uri", c.Request().RequestURI),
	).Sugar()
}
