package admin

import (
	"errors"
	"net/http"
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

// Logout documents administrator logout.
//
// @Summary Log out an administrator
// @Tags Admin - Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "LOGOUT_FAILED", "logout", false)(w, r)
}

// ListTransactions documents transaction administration.
//
// @Summary List transactions
// @Tags Admin - Transactions
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/transactions [get]
func (h *Handler) ListTransactions(w http.ResponseWriter, r *http.Request) {
	h.List("transactions")(w, r)
}

// ListAuditLogs documents audit log administration.
//
// @Summary List audit logs
// @Tags Admin - Audit
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/audit-logs [get]
func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) { h.List("audit")(w, r) }

// ListProviderHealth documents provider health administration.
//
// @Summary List provider health snapshots
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/health/providers [get]
func (h *Handler) ListProviderHealth(w http.ResponseWriter, r *http.Request) { h.List("health")(w, r) }

// ListAdmins documents administrator listing.
//
// @Summary List administrators
// @Tags Admin - Users
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/users [get]
func (h *Handler) ListAdmins(w http.ResponseWriter, r *http.Request) { h.List("admins")(w, r) }

// CreateAdminUser documents administrator creation.
//
// @Summary Create an administrator
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateAdminRequest true "Administrator"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users [post]
func (h *Handler) CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	h.Action(201, "ADMIN_CREATE_FAILED", "admin.create", true)(w, r)
}

// ChangeAdminPassword documents administrator password changes.
//
// @Summary Change an administrator password
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Administrator ID"
// @Param request body apidoc.ChangePasswordRequest true "New password"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/password [patch]
func (h *Handler) ChangeAdminPassword(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "PASSWORD_CHANGE_FAILED", "admin.password", true)(w, r)
}

// ChangeAdminStatus documents administrator status changes.
//
// @Summary Change an administrator status
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Administrator ID"
// @Param request body apidoc.ChangeStatusRequest true "Status" SchemaExample({"status":"active"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/status [patch]
func (h *Handler) ChangeAdminStatus(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "STATUS_CHANGE_FAILED", "admin.status", true)(w, r)
}

// ListApps documents application listing.
//
// @Summary List applications
// @Tags Admin - Applications
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/apps [get]
func (h *Handler) ListApps(w http.ResponseWriter, r *http.Request) { h.List("apps")(w, r) }

// CreateApp documents application creation.
//
// @Summary Create an application
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateAppRequest true "Application"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps [post]
func (h *Handler) CreateApp(w http.ResponseWriter, r *http.Request) {
	h.Action(201, "APP_CREATE_FAILED", "app.create", true)(w, r)
}

// UpdateApp documents application updates.
//
// @Summary Update an application
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.UpdateAppRequest true "Application changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id} [patch]
func (h *Handler) UpdateApp(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "APP_UPDATE_FAILED", "app.update", true)(w, r)
}

// ChangeAppStatus documents application status changes.
//
// @Summary Change application status
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.ChangeStatusRequest true "Status" SchemaExample({"status":"disabled"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/status [patch]
func (h *Handler) ChangeAppStatus(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "APP_STATUS_CHANGE_FAILED", "app.status", true)(w, r)
}

// ListCredentials documents application credential listing.
//
// @Summary List application credentials
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials [get]
func (h *Handler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	h.List("credentials")(w, r)
}

// CreateCredential documents application credential creation.
//
// @Summary Create an application credential
// @Description Returns the client secret once. Store it securely because it cannot be retrieved later.
// @Tags Admin - Credentials
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param request body apidoc.CreateCredentialRequest true "Credential"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials [post]
func (h *Handler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	h.Action(201, "APP_CREDENTIAL_CREATE_FAILED", "credential.create", true)(w, r)
}

// RevokeCredential documents credential revocation.
//
// @Summary Revoke an application credential
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param credentialID path string true "Credential ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials/{credentialID}/revoke [patch]
func (h *Handler) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "APP_CREDENTIAL_REVOKE_FAILED", "credential.revoke", false)(w, r)
}

// RotateCredential documents credential rotation.
//
// @Summary Rotate an application credential
// @Description Replaces the client secret, revokes existing sessions, and returns the new secret once.
// @Tags Admin - Credentials
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Param credentialID path string true "Credential ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials/{credentialID}/rotate [post]
func (h *Handler) RotateCredential(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "APP_CREDENTIAL_ROTATE_FAILED", "credential.rotate", false)(w, r)
}

// ListProviders documents provider account listing.
//
// @Summary List provider accounts
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/providers [get]
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) { h.List("providers")(w, r) }

// CreateProvider documents provider account creation.
//
// @Summary Create a provider account
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateProviderAccountRequest true "Provider account"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts [post]
func (h *Handler) CreateProvider(w http.ResponseWriter, r *http.Request) {
	h.Action(201, "PROVIDER_CREATE_FAILED", "provider.create", true)(w, r)
}

// UpdateProviderCountries documents provider country updates.
//
// @Summary Update provider countries
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param request body apidoc.UpdateCountriesRequest true "Countries"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/countries [patch]
func (h *Handler) UpdateProviderCountries(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "COUNTRIES_UPDATE_FAILED", "provider.countries", true)(w, r)
}

// UpdateProviderConfig documents provider configuration updates.
//
// @Summary Update provider configuration
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param request body apidoc.UpdateProviderConfigRequest true "Provider configuration"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/config [patch]
func (h *Handler) UpdateProviderConfig(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "CONFIG_UPDATE_FAILED", "provider.config", true)(w, r)
}

// ActivateProvider documents provider activation.
//
// @Summary Activate a provider account
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/activate [patch]
func (h *Handler) ActivateProvider(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "PROVIDER_ACTIVATE_FAILED", "provider.activate", false)(w, r)
}

// DeactivateProvider documents provider deactivation.
//
// @Summary Deactivate a provider account
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/deactivate [patch]
func (h *Handler) DeactivateProvider(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "PROVIDER_DEACTIVATE_FAILED", "provider.deactivate", false)(w, r)
}

// TestProvider documents provider configuration tests.
//
// @Summary Test provider configuration
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/test [post]
func (h *Handler) TestProvider(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "PROVIDER_TEST_FAILED", "provider.test", false)(w, r)
}

// ListRoutes documents payment route listing.
//
// @Summary List payment routes
// @Tags Admin - Routes
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 500 {object} apidoc.ErrorResponse
// @Router /api/admin/routes [get]
func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) { h.List("routes")(w, r) }

// CreateRoute documents payment route creation.
//
// @Summary Create a payment route
// @Tags Admin - Routes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body apidoc.CreateRouteRequest true "Payment route"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes [post]
func (h *Handler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	h.Action(201, "ROUTE_CREATE_FAILED", "route.create", true)(w, r)
}

// UpdateRoute documents payment route updates.
//
// @Summary Update a payment route
// @Tags Admin - Routes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Payment route ID"
// @Param request body apidoc.UpdateRouteRequest true "Route changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes/{id} [patch]
func (h *Handler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	h.Action(200, "ROUTE_UPDATE_FAILED", "route.update", true)(w, r)
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
