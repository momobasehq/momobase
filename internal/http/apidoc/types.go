// Package apidoc contains schemas used only to describe the HTTP API.
package apidoc

import (
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/providers"
)

// Response describes the successful JSON envelope returned by the API.
type Response[T any] struct {
	Success bool               `json:"success" example:"true"`
	Data    T                  `json:"data"`
	Error   *platform.APIError `json:"error,omitempty"`
	Message string             `json:"message,omitempty"`
}

// DocResponse is a swagger-friendly success envelope.
type DocResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    interface{}        `json:"data"`
	Error   *platform.APIError `json:"error,omitempty"`
	Message string             `json:"message,omitempty"`
}

// TokenResponse is a swagger-friendly token pair response.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ErrorResponse describes the JSON envelope returned for an API error.
type ErrorResponse struct {
	Success bool               `json:"success" example:"false"`
	Error   *platform.APIError `json:"error"`
	Message string             `json:"message" example:"request failed"`
}

// Page describes paginated API data.
type Page[T any] struct {
	Page  int `json:"page" example:"1"`
	Total int `json:"total" example:"1"`
	Items []T `json:"items"`
	Count int `json:"count" example:"1"`
}

// OK is returned by successful operations that have no resource body.
type OK struct {
	OK bool `json:"ok" example:"true"`
}

// Health is returned by the lightweight health endpoint.
type Health struct {
	OK bool `json:"ok" example:"true"`
}

// ProviderRegistry lists the provider codes registered in the running build.
type ProviderRegistry struct {
	Providers []string `json:"providers" example:"dummy"`
}

// ProviderRegistryResponse is a swagger-friendly registered-provider response.
type ProviderRegistryResponse struct {
	Success bool               `json:"success" example:"true"`
	Data    ProviderRegistry   `json:"data"`
	Error   *platform.APIError `json:"error,omitempty"`
	Message string             `json:"message,omitempty"`
}

// SystemInfo describes the running service.
type SystemInfo struct {
	AppName        string    `json:"app_name" example:"momobase"`
	AppEnv         string    `json:"app_env" example:"production"`
	DBType         string    `json:"db_type" example:"postgres"`
	Addr           string    `json:"addr" example:":9090"`
	WorkersEnabled bool      `json:"workers_enabled" example:"true"`
	WorkerNames    []string  `json:"worker_names" example:"health,reconciliation"`
	GoVersion      string    `json:"go_version" example:"go1.23.0"`
	ServerTime     time.Time `json:"server_time"`
}

// SystemHealth describes database and runtime health.
type SystemHealth struct {
	OK                         bool      `json:"ok" example:"true"`
	Database                   string    `json:"database" enums:"ok,error" example:"ok"`
	RuntimeProviderCount       int       `json:"runtime_provider_count" example:"2"`
	ActiveProviderAccountCount int64     `json:"active_provider_account_count" example:"2"`
	WorkersConfigured          []string  `json:"workers_configured" example:"health,reconciliation"`
	ServerTime                 time.Time `json:"server_time"`
}

// Worker describes a configured background worker.
type Worker struct {
	Name       string `json:"name" example:"health"`
	Configured bool   `json:"configured" example:"true"`
	State      string `json:"state" example:"managed_by_single_binary"`
}

// Runtime describes an initialized provider runtime.
type Runtime struct {
	ProviderAccountID string                         `json:"provider_account_id" example:"pacc_123"`
	ProviderCode      string                         `json:"provider_code" example:"dummy"`
	ConfigVersion     int                            `json:"config_version" example:"1"`
	Active            bool                           `json:"active" example:"true"`
	Initialized       bool                           `json:"initialized" example:"true"`
	Capabilities      []providers.Capability         `json:"capabilities"`
	Countries         []string                       `json:"countries" example:"UG"`
	Health            *domain.ProviderHealthSnapshot `json:"health,omitempty"`
}
