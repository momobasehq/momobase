package admin

import (
	"errors"
	"net/http"
	"runtime"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

// SystemInfo describes application and worker metadata exposed by the
// administrative system endpoints.
type SystemInfo struct {
	AppName        string
	AppEnv         string
	DBType         string
	Addr           string
	WorkersEnabled bool
	WorkerNames    []string
}

// Handler serves authenticated administrative HTTP endpoints.
type Handler struct {
	db        *gorm.DB
	auth      *services.AdminAuthService
	users     *services.AdminUserService
	providers *services.ProviderAdminService
	routes    *services.RouteAdminService
	apps      *services.AppService
	runtime   *services.ProviderRuntimeManager
	audit     *services.AuditService
	system    SystemInfo
}

// NewHandler constructs an administrative HTTP handler from its services and
// system metadata.
func NewHandler(
	db *gorm.DB,
	auth *services.AdminAuthService,
	users *services.AdminUserService,
	providers *services.ProviderAdminService,
	routes *services.RouteAdminService,
	apps *services.AppService,
	runtime *services.ProviderRuntimeManager,
	audit *services.AuditService,
	system SystemInfo,
) *Handler {
	return &Handler{db, auth, users, providers, routes, apps, runtime, audit, system}
}

// Request contains the fields accepted by administrative mutation endpoints.
// Individual actions use only the fields relevant to that action.
type Request struct {
	Name              string         `json:"name"`
	Email             string         `json:"email"`
	Password          string         `json:"password"`
	Role              string         `json:"role"`
	Status            string         `json:"status"`
	Description       string         `json:"description"`
	Environment       string         `json:"environment"`
	Scopes            string         `json:"scopes"`
	ExpiresAt         string         `json:"expires_at"`
	ProviderCode      string         `json:"provider_code"`
	Countries         []string       `json:"countries"`
	ServiceType       string         `json:"service_type"`
	PaymentMethod     string         `json:"payment_method"`
	ProviderAccountID string         `json:"provider_account_id"`
	Config            map[string]any `json:"config"`
	Priority          int            `json:"priority"`
	Active            bool           `json:"active"`
}

func actor(r *http.Request) *domain.AdminUser { return authmw.AdminUser(r) }
func id(r *http.Request) string               { return r.PathValue("id") }
func reply(w http.ResponseWriter, status int, code string, run func() (any, error)) {
	out, err := run()
	if err != nil {
		platform.Error(w, 400, code, err.Error())
		return
	}
	if out == nil {
		out = map[string]bool{"ok": true}
	}
	platform.JSON(w, status, out)
}

// Action returns an HTTP handler that optionally decodes a Request, dispatches
// the named administrative action, and writes the configured success or error
// response.
func (h *Handler) Action(status int, code, action string, decode bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request *Request
		if decode {
			var err error
			request, err = platform.DecodeJSON[Request](r)
			if err != nil {
				platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
				return
			}
		}
		reply(w, status, code, func() (any, error) { return h.dispatch(r, request, action) })
	}
}
func (h *Handler) dispatch(r *http.Request, v *Request, action string) (any, error) {
	a, target := actor(r), id(r)
	switch action {
	case "logout":
		return nil, h.auth.LogoutBearer(r.Context(), authmw.BearerToken(r), r.RemoteAddr, r.UserAgent())
	case "admin.create":
		return h.users.Create(r.Context(), a, v.Name, v.Email, v.Password, v.Role)
	case "admin.password":
		return nil, h.users.ChangePassword(r.Context(), a, target, v.Password)
	case "admin.status":
		return nil, h.users.ChangeStatus(r.Context(), a, target, v.Status)
	case "app.create":
		return h.apps.CreateApp(r.Context(), a, v.Name, v.Description, v.Environment)
	case "app.update":
		return h.apps.UpdateApp(r.Context(), a, target, v.Name, v.Description, v.Environment)
	case "app.status":
		return nil, h.apps.ChangeAppStatus(r.Context(), a, target, v.Status)
	case "credential.create":
		expires, err := parseExpiry(v.ExpiresAt)
		if err != nil {
			return nil, err
		}
		return h.apps.CreateCredential(r.Context(), a, target, v.Name, v.Scopes, expires)
	case "credential.revoke":
		return nil, h.apps.RevokeCredential(r.Context(), a, target, r.PathValue("credentialID"))
	case "credential.rotate":
		return h.apps.RotateCredential(r.Context(), a, target, r.PathValue("credentialID"))
	case "provider.create":
		return h.providers.CreateAccount(r.Context(), a, v.ProviderCode, v.Name, v.Environment, v.Countries, v.Config)
	case "provider.countries":
		return nil, h.providers.UpdateCountries(r.Context(), a, target, v.Countries)
	case "provider.config":
		return nil, h.providers.UpdateConfig(r.Context(), a, target, v.Config)
	case "provider.activate":
		return nil, h.providers.Activate(r.Context(), a, target)
	case "provider.deactivate":
		return nil, h.providers.Deactivate(r.Context(), a, target)
	case "provider.test":
		err := h.runtime.TestProviderConfig(r.Context(), target)
		if err == nil {
			h.audit.RecordBestEffort(r.Context(), a.ID, "admin", "provider.tested", "provider_account", target, nil, r.RemoteAddr, r.UserAgent())
		}
		return nil, err
	case "route.create":
		return h.routes.Create(r.Context(), a, v.ServiceType, v.PaymentMethod, v.ProviderAccountID, v.Priority, v.Active)
	case "route.update":
		return nil, h.routes.Update(r.Context(), a, target, v.Priority, v.Active)
	}
	return nil, errors.New("unsupported admin action")
}
func parseExpiry(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, errors.New("expires_at must be RFC3339")
	}
	return &value, nil
}

// List returns an HTTP handler that writes a paginated list for the requested
// administrative resource kind.
func (h *Handler) List(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := h.db.WithContext(r.Context())
		switch kind {
		case "admins":
			page[domain.AdminUser](w, r, db.Model(&domain.AdminUser{}), "created_at desc")
		case "apps":
			page[domain.App](w, r, db.Model(&domain.App{}), "created_at desc")
		case "credentials":
			page[domain.AppCredential](w, r, db.Model(&domain.AppCredential{}).Where("app_id = ?", id(r)), "created_at desc")
		case "providers":
			page[domain.ProviderAccount](w, r, db.Model(&domain.ProviderAccount{}), "created_at desc")
		case "routes":
			page[domain.PaymentRoute](w, r, db.Model(&domain.PaymentRoute{}), "service_type asc, payment_method asc, priority asc")
		case "health":
			page[domain.ProviderHealthSnapshot](w, r, db.Model(&domain.ProviderHealthSnapshot{}), "updated_at desc")
		case "transactions":
			page[domain.Transaction](w, r, db.Model(&domain.Transaction{}), "created_at desc")
		case "audit":
			page[domain.AuditLog](w, r, db.Model(&domain.AuditLog{}), "created_at desc")
		}
	}
}
func page[T any](w http.ResponseWriter, r *http.Request, query *gorm.DB, order string) {
	page, size := platform.Pagination(r)
	var total int64
	var items []T
	err := query.Count(&total).Error
	if err == nil {
		err = query.Order(order).Limit(size).Offset((page - 1) * size).Find(&items).Error
	}
	if err != nil {
		platform.Error(w, 500, "SERVER_ERROR", err.Error())
		return
	}
	platform.JSON(w, 200, platform.PageData[T]{Page: page, Total: int(total), Items: items, Count: len(items)})
}

// Me writes the authenticated administrator stored in the request context.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) { platform.JSON(w, 200, actor(r)) }

// GetApp writes the application identified by the request path or a not-found
// response when no matching application exists.
func (h *Handler) GetApp(w http.ResponseWriter, r *http.Request) {
	app, err := h.apps.GetApp(r.Context(), id(r))
	if err != nil {
		platform.Error(w, 404, "NOT_FOUND", "app not found")
		return
	}
	platform.JSON(w, 200, app)
}

// ActiveProviderBalances queries balances for active provider runtimes and
// writes them as a paginated response.
func (h *Handler) ActiveProviderBalances(w http.ResponseWriter, r *http.Request) {
	items, err := h.runtime.QueryActiveBalances(r.Context())
	if err != nil {
		platform.Error(w, 400, "BALANCE_QUERY_FAILED", err.Error())
		return
	}
	h.audit.RecordBestEffort(
		r.Context(),
		actor(r).ID,
		"admin",
		"balances.active_providers_queried",
		"provider_account",
		"all_active",
		nil,
		r.RemoteAddr,
		r.UserAgent(),
	)
	page, size := platform.Pagination(r)
	platform.JSON(w, 200, platform.PaginateSlice(items, page, size))
}

// ProviderBalance queries and writes the balance for the provider account and
// optional country identified by the request.
func (h *Handler) ProviderBalance(w http.ResponseWriter, r *http.Request) {
	out, err := h.runtime.QueryBalance(r.Context(), id(r), r.URL.Query().Get("country"))
	if err != nil {
		platform.Error(w, 400, "BALANCE_QUERY_FAILED", err.Error())
		return
	}
	h.audit.RecordBestEffort(r.Context(), actor(r).ID, "admin", "balance.queried", "provider_account", id(r), nil, r.RemoteAddr, r.UserAgent())
	platform.JSON(w, 200, out)
}

// SystemInfo writes application, runtime, and worker metadata.
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
