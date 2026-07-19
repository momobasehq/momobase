package admin

import (
	"errors"
	"net/http"
	"runtime"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"gorm.io/gorm"
)

// SystemInfo writes application, runtime, and worker metadata.
//
// @Summary Get system information
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.SystemInfo
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/system/info [get]
func (h *Handler) SystemInfo(w http.ResponseWriter, _ *http.Request) {
	platform.JSON(w, 200, map[string]any{
		"app_name":        h.system.AppName,
		"app_env":         h.system.AppEnv,
		"db_type":         h.system.DBType,
		"addr":            h.system.Addr,
		"workers_enabled": h.system.WorkersEnabled,
		"worker_names":    h.system.WorkerNames,
		"go_version":      runtime.Version(),
		"server_time":     time.Now().UTC(),
	})
}

// SystemHealth checks database connectivity and writes system and provider
// runtime health information.
//
// @Summary Get system health
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.SystemHealth
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/system/health [get]
func (h *Handler) SystemHealth(w http.ResponseWriter, r *http.Request) {
	sqlDB, err := h.db.DB()
	if err != nil {
		platform.Error(w, 500, "DB_ERROR", err.Error())
		return
	}
	dbOK := sqlDB.PingContext(r.Context()) == nil
	var active int64
	if err = h.db.WithContext(r.Context()).Model(&domain.ProviderAccount{}).Where("active = ?", true).Count(&active).Error; err != nil {
		platform.Error(w, 500, "DB_ERROR", err.Error())
		return
	}
	status := "error"
	if dbOK {
		status = "ok"
	}
	platform.JSON(w, 200, map[string]any{
		"ok":                            dbOK,
		"database":                      status,
		"runtime_provider_count":        len(h.runtime.List()),
		"active_provider_account_count": active,
		"workers_configured":            h.system.WorkerNames,
		"server_time":                   time.Now().UTC(),
	})
}

// Workers writes a paginated view of the configured background workers.
//
// @Summary List configured workers
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/workers [get]
func (h *Handler) Workers(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 0, len(h.system.WorkerNames))
	for _, name := range h.system.WorkerNames {
		items = append(items, map[string]any{
			"name":       name,
			"configured": true,
			"state":      "managed_by_single_binary",
		})
	}
	page, size := platform.Pagination(r)
	platform.JSON(w, 200, platform.PaginateSlice(items, page, size))
}

// RuntimeProviders writes a paginated view of initialized provider runtimes
// together with their latest health snapshots when available.
//
// @Summary List provider runtimes
// @Tags Admin - System
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/runtime/providers [get]
func (h *Handler) RuntimeProviders(w http.ResponseWriter, r *http.Request) {
	items := make([]map[string]any, 0)
	for _, runtime := range h.runtime.List() {
		item := map[string]any{
			"provider_account_id": runtime.AccountID,
			"provider_code":       runtime.ProviderCode,
			"config_version":      runtime.ConfigVersion,
			"active":              true,
			"initialized":         true,
			"capabilities":        runtime.Capabilities,
			"countries":           runtime.Countries,
		}
		var health domain.ProviderHealthSnapshot
		if err := h.db.WithContext(r.Context()).First(&health, "provider_account_id = ?", runtime.AccountID).Error; err == nil {
			item["health"] = &health
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			platform.Error(w, 500, "SERVER_ERROR", err.Error())
			return
		}
		items = append(items, item)
	}
	page, size := platform.Pagination(r)
	platform.JSON(w, 200, platform.PaginateSlice(items, page, size))
}
