package model

import (
	"encoding/json"
	"time"
)

// User struct for user related requests
type User struct {
	ID        int64     `json:"id"`
	EmailID   string    `json:"email_id"`
	UserName  string    `json:"user_name"`
	Password  string    `json:"password"`
	CreatedAt time.Time `json:"created_at"`
}

// Service struct for service related requests
type Service struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	NumberOfVersions int64     `json:"number_of_versions"`
	Tags             []string  `json:"tags"`
	CreatedBy        int64     `json:"created_by"`
	UpdatedBy        int64     `json:"updated_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// AuditEntry is a single record from service_audit_log.
type AuditEntry struct {
	ID        int64           `json:"id"`
	ServiceID int64           `json:"service_id"`
	Action    string          `json:"action"` // "create" | "update" | "delete"
	ChangedBy int64           `json:"changed_by"`
	ChangedAt time.Time       `json:"changed_at"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

// Version struct for service version related responses
type Version struct {
	ID            int64     `json:"id"`
	ServiceID     int64     `json:"service_id,omitempty"`
	VersionString string    `json:"version_string,omitempty"`
	Status        string    `json:"status,omitempty"`
	CreatedBy     int64     `json:"created_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// CreateServiceRequest is the payload to create a service and initial version
type CreateServiceRequest struct {
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Tags        []string             `json:"tags"`
	Version     CreateVersionRequest `json:"version"`
	CreatedBy   int64                `json:"-"`
}

// CreateVersionRequest is the nested payload for creating a service version
type CreateVersionRequest struct {
	VersionString string `json:"version_string"`
	Status        string `json:"status"`
	CreatedBy     int64  `json:"-"`
}

// UpdateServiceRequest is the payload to update a service
type UpdateServiceRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	UpdatedBy   int64    `json:"-"`
}

// PatchServiceRequest is the payload for partial update of service
type PatchServiceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	UpdatedBy   int64   `json:"-"`
}

// SignUpRequest is the payload for signup request
type SignUpRequest struct {
	EmailID  string `json:"email_id"`
	UserName string `json:"user_name"`
	Password string `json:"password"`
}

// LoginRequest is the payload for login request
type LoginRequest struct {
	EmailID  string `json:"email_id"`
	Password string `json:"password"`
}

// UserResponse is the JSON response for User.
type UserResponse struct {
	ID        int64  `json:"id"`
	EmailID   string `json:"email_id"`
	UserName  string `json:"user_name"`
	CreatedAt string `json:"created_at"`
}

// ServiceSearchParams contains params for searching services
type ServiceSearchParams struct {
	Name        string `query:"name"`
	Description string `query:"description"`
	Tag         string `query:"tag"` // filter to services that have this tag
	Page        int    `query:"page"`
	PageSize    int    `query:"page_size"`
	SortBy      string `query:"sort_by"` // "name" or "created_at"
	Order       string `query:"order"`   // "asc" or "desc"
}

// VersionSearchParams contains params for searching service versions
type VersionSearchParams struct {
	Status    string `query:"status"`
	CreatedBy int    `query:"created_by"`
	Page      int    `query:"page"`
	PageSize  int    `query:"page_size"`
}

// PaginationMeta carries pagination info alongside a list response.
type PaginationMeta struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// TransformToUserResponse converts User to a password-safe response.
func TransformToUserResponse(user User) UserResponse {
	return UserResponse{
		ID:        user.ID,
		EmailID:   user.EmailID,
		UserName:  user.UserName,
		CreatedAt: user.CreatedAt.Format(time.RFC3339),
	}
}
