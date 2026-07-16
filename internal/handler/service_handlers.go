package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	authmw "github.com/souvikmndl/search-service/internal/middleware"
	"github.com/souvikmndl/search-service/internal/model"
)

// CreateService creates a new service
func (h *ServiceHandler) CreateService(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	var req model.CreateServiceRequest
	if err := c.Bind(&req); err != nil {
		logger.Errorf("error decoding request body %v", err)
		return h.badRequestErrorResponse(c)
	}

	var createSvcErrs []ValidationError
	if len(req.Name) < 3 {
		createSvcErrs = append(createSvcErrs, ValidationError{Field: "name", Message: "must be at least 3 characters"})
	}
	if req.Version.VersionString == "" {
		createSvcErrs = append(createSvcErrs, ValidationError{Field: "version.version_string", Message: "is required"})
	}
	if req.Version.Status == "" {
		createSvcErrs = append(createSvcErrs, ValidationError{Field: "version.status", Message: "is required"})
	}
	if len(createSvcErrs) > 0 {
		logger.Error("create service validation failed")
		return h.validationErrorResponse(c, createSvcErrs)
	}

	userID, err := authenticatedUserID(c)
	if err != nil {
		logger.Errorf("unable to get authenticated user id %v", err)
		return h.serverErrorResponse(c)
	}
	req.CreatedBy = userID

	svc, err := h.SearchDB.CreateService(c.Request().Context(), req)
	if err != nil {
		logger.Errorf("unable to save service to db %v", err)
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return h.duplicateTitleErrResponse(c)
		}
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusCreated, envelope{"data": svc})
}

// Search is the func for Search API
func (h *ServiceHandler) Search(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	var params model.ServiceSearchParams

	if err := c.Bind(&params); err != nil {
		logger.Errorf("invalid query params %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid query parameters format",
		})
	}

	services, total, err := h.SearchDB.SearchServices(c.Request().Context(), params)
	if err != nil {
		logger.Errorf("unable to search services %v", err)
		return h.serverErrorResponse(c)
	}

	// Mirror the clamping the DB layer applies so the meta is consistent.
	pageSize := params.PageSize
	if pageSize < 10 || pageSize > 20 {
		pageSize = 10
	}
	page := params.Page
	if page < 1 {
		page = 1
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}

	resp := make([]model.Service, 0, len(services))
	resp = append(resp, services...)

	return c.JSON(http.StatusOK, envelope{
		"data": resp,
		"meta": model.PaginationMeta{
			Page:       page,
			PageSize:   pageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// GetServiceByID returns a single service by id
func (h *ServiceHandler) GetServiceByID(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.Atoi(svcID)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	logger.Infof("Service ID %d", id)

	svc, err := h.SearchDB.GetServiceByID(c.Request().Context(), int64(id))
	if err != nil {
		logger.Errorf("unable to fetch service by id: %d, error: %v", id, err)
		if errors.Is(err, sql.ErrNoRows) {
			return h.notFoundErrResponse(c)
		}

		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusOK, envelope{"data": svc})
}

// CreateServiceVersion creates a new version for an existing service
func (h *ServiceHandler) CreateServiceVersion(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.Atoi(svcID)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	logger.Infof("Service ID %d", id)

	var req model.CreateVersionRequest
	err = c.Bind(&req)
	if err != nil {
		logger.Errorf("error decoding request body %v", err)
		return h.badRequestErrorResponse(c)
	}

	var versionErrs []ValidationError
	if req.VersionString == "" {
		versionErrs = append(versionErrs, ValidationError{Field: "version_string", Message: "is required"})
	}
	if req.Status == "" {
		versionErrs = append(versionErrs, ValidationError{Field: "status", Message: "is required"})
	}
	if len(versionErrs) > 0 {
		logger.Error("create version validation failed")
		return h.validationErrorResponse(c, versionErrs)
	}

	userID, err := authenticatedUserID(c)
	if err != nil {
		logger.Errorf("unable to get authenticated user id %v", err)
		return h.serverErrorResponse(c)
	}
	req.CreatedBy = userID

	version, err := h.SearchDB.CreateServiceVersion(c.Request().Context(), int64(id), req)
	if err != nil {
		logger.Errorf("unable to create new service version %v", err)
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusCreated, envelope{"data": version})
}

// GetServiceVersions returns all versions for a service.
func (h *ServiceHandler) GetServiceVersions(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.Atoi(svcID)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	if _, err := h.SearchDB.GetServiceByID(c.Request().Context(), int64(id)); err != nil {
		logger.Errorf("unable to fetch service by id: %d, error: %v", id, err)
		if errors.Is(err, sql.ErrNoRows) {
			return h.notFoundErrResponse(c)
		}

		return h.serverErrorResponse(c)
	}

	versions, err := h.SearchDB.GetServiceVersions(c.Request().Context(), int64(id))
	if err != nil {
		logger.Errorf("unable to fetch service versionsid: %d, error: %v", id, err)
		return h.serverErrorResponse(c)
	}

	resp := make([]model.Version, 0, len(versions))
	resp = append(resp, versions...)

	return c.JSON(http.StatusOK, envelope{"data": resp})
}

// UpdateServiceByID updates name/description of an existing service
func (h *ServiceHandler) UpdateServiceByID(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.Atoi(svcID)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	logger.Infof("Service ID %d", id)

	var req model.UpdateServiceRequest
	if err := c.Bind(&req); err != nil {
		logger.Errorf("error decoding request body %v", err)
		return h.badRequestErrorResponse(c)
	}

	if len(req.Name) < 3 {
		logger.Error("update service validation failed")
		return h.validationErrorResponse(c, []ValidationError{
			{Field: "name", Message: "must be at least 3 characters"},
		})
	}

	svc, err := h.SearchDB.GetServiceByID(c.Request().Context(), int64(id))
	if err != nil {
		logger.Errorf("unable to fetch service by id %v", err)
		if errors.Is(err, sql.ErrNoRows) {
			return h.notFoundErrResponse(c)
		}

		return h.serverErrorResponse(c)
	}

	svc.Name = req.Name
	svc.Description = req.Description
	svc.Tags = req.Tags
	userID, err := authenticatedUserID(c)
	if err != nil {
		logger.Errorf("unable to get authenticated user id %v", err)
		return h.serverErrorResponse(c)
	}
	svc.UpdatedBy = userID

	if err := h.SearchDB.UpdateService(c.Request().Context(), svc); err != nil {
		logger.Errorf("unable to save service to db %v", err)
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return h.duplicateTitleErrResponse(c)
		}
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusOK, envelope{"data": svc})
}

// DeleteServiceByID deletes an existing service by id
func (h *ServiceHandler) DeleteServiceByID(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.Atoi(svcID)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	logger.Infof("Service ID %d", id)

	userID, err := authenticatedUserID(c)
	if err != nil {
		logger.Errorf("unable to get authenticated user id %v", err)
		return h.serverErrorResponse(c)
	}

	err = h.SearchDB.DeleteServiceByID(c.Request().Context(), int64(id), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("service with given id does not exist id: %d error %v", id, err)
			return h.notFoundErrResponse(c)
		}
		logger.Errorf("unable to delete service from db %v", err)
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusOK, "ok")
}

func authenticatedUserID(c echo.Context) (int64, error) {
	claims, ok := c.Get("user").(*authmw.Claims)
	if !ok || claims == nil || claims.UserID == 0 {
		return 0, fmt.Errorf("authenticated user claims missing")
	}

	return claims.UserID, nil
}

func (h *ServiceHandler) PatchService(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)
	logger.Info("Patch Service API")

	svcID := c.Param("id")
	id, err := strconv.ParseInt(svcID, 10, 64)
	if err != nil {
		logger.Errorf("invalid service id %+v", err)
		return h.invalidIDErrResponse(c)
	}

	svc, err := h.SearchDB.GetServiceByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("service with given id does doesnot exist, id: %d, error: %+v", id, err)
			return h.notFoundErrResponse(c)
		}
		logger.Errorf("unable to fetch serivice with id: %d, error: %+v", id, err)
		return h.serverErrorResponse(c)
	}

	var req model.PatchServiceRequest
	err = c.Bind(&req)
	if err != nil {
		logger.Errorf("unable to bind request body %+v", err)
		return h.badRequestErrorResponse(c)
	}

	if req.Name != nil {
		if len(*req.Name) < 3 || len(*req.Name) > 40 {
			logger.Error("invalid length of name")
			return h.validationErrorResponse(c, []ValidationError{
				{Field: "name", Message: "name must be between 3 and 40 characters"},
			})
		}
		svc.Name = *req.Name
	}

	if req.Description != nil {
		svc.Description = *req.Description
	}

	userID, err := authenticatedUserID(c)
	if err != nil {
		logger.Errorf("unable to get authenticated user id %v", err)
		return h.serverErrorResponse(c)
	}

	svc.UpdatedBy = userID

	err = h.SearchDB.UpdateService(c.Request().Context(), svc)
	if err != nil {
		logger.Errorf("error updating service in db %+v", err)
		return h.serverErrorResponse(c)
	}

	return c.JSON(http.StatusOK, envelope{"data": svc})
}

// GetServiceAuditLog returns the audit history for a service.
func (h *ServiceHandler) GetServiceAuditLog(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)

	svcID := c.Param("id")
	id, err := strconv.ParseInt(svcID, 10, 64)
	if err != nil {
		logger.Error("invalid service id")
		return h.invalidIDErrResponse(c)
	}

	entries, err := h.SearchDB.GetAuditLog(c.Request().Context(), id)
	if err != nil {
		logger.Errorf("unable to fetch audit log for service %d: %v", id, err)
		return h.serverErrorResponse(c)
	}

	resp := make([]model.AuditEntry, 0, len(entries))
	resp = append(resp, entries...)

	return c.JSON(http.StatusOK, envelope{"data": resp})
}

func (h *ServiceHandler) SearchVersions(c echo.Context) error {
	logger := LoggerWithFields(h.Logger, c)
	logger.Info("Search Service Versions")

	svcID := c.Param("id")
	id, err := strconv.ParseInt(svcID, 10, 64)
	if err != nil {
		logger.Errorf("error parsing service id %+v", err)
		return h.invalidIDErrResponse(c)
	}

	svc, err := h.SearchDB.GetServiceByID(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			logger.Errorf("service with given id: %d does not exist", id)
			return h.notFoundErrResponse(c)
		}
		logger.Errorf("error fetching service by id %+v", err)
		return h.serverErrorResponse(c)
	}

	logger.Infof("Service: %+v", svc)

	var req model.VersionSearchParams
	err = c.Bind(&req)
	if err != nil {
		logger.Errorf("invalid request %+v", err)
		return h.badRequestErrorResponse(c)
	}

	logger.Infof("search params %+v", req)

	versions, total, err := h.SearchDB.SearchVersions(c.Request().Context(), id, req)
	if err != nil {
		logger.Errorf("error searching for versions %+v", err)
		return h.serverErrorResponse(c)
	}

	if req.PageSize < 10 || req.PageSize > 20 {
		req.PageSize = 10
	}

	totalPages := 0
	if total > 0 {
		totalPages = (int(total) + req.PageSize - 1) / req.PageSize
	}

	return c.JSON(http.StatusOK, envelope{
		"data": versions,
		"meta": model.PaginationMeta{
			Page:       req.Page,
			PageSize:   req.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
