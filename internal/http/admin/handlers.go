package admin

import (
	"errors"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/http/apidoc"
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
	audit     *audit.Service
	authz     *services.AuthzService
	analytics *services.AnalyticsService
	system    SystemInfo
}

type Response interface{}

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
	audit *audit.Service,
	authz *services.AuthzService,
	analytics *services.AnalyticsService,
	system SystemInfo,
) *Handler {
	return &Handler{db, auth, users, providers, routes, apps, runtime, audit, authz, analytics, system}
}

func actor(r *http.Request) *domain.AdminUser { return authmw.AdminUser(r) }
func id(r *http.Request) string               { return r.PathValue("id") }

// reply is a generic helper for handling HTTP responses.
func reply[T Response](w http.ResponseWriter, status int, code string, run func() (T, error)) {
	out, err := run()
	if err != nil {
		platform.Error(w, 400, code, err.Error())
		return
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
	reply(w, 200, "LOGOUT_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.auth.LogoutBearer(r.Context(), authmw.BearerToken(r), r.RemoteAddr, r.UserAgent())
	})
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
	page[domain.Transaction](w, r, h.db.WithContext(r.Context()).Model(&domain.Transaction{}), "created_at desc")
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
func (h *Handler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	page[domain.AuditLog](w, r, h.db.WithContext(r.Context()).Model(&domain.AuditLog{}), "created_at desc")
}

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
func (h *Handler) ListProviderHealth(w http.ResponseWriter, r *http.Request) {
	page[domain.ProviderHealthSnapshot](w, r, h.db.WithContext(r.Context()).Model(&domain.ProviderHealthSnapshot{}), "updated_at desc")
}

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
func (h *Handler) ListAdmins(w http.ResponseWriter, r *http.Request) {
	page[domain.AdminUser](w, r, h.db.WithContext(r.Context()).Model(&domain.AdminUser{}), "created_at desc")
}

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
	req, err := platform.DecodeJSON[apidoc.CreateAdminRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 201, "ADMIN_CREATE_FAILED", func() (*domain.AdminUser, error) {
		return h.users.Create(r.Context(), actor(r), req.Name, req.Email, req.Password, req.Role)
	})
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
	req, err := platform.DecodeJSON[apidoc.ChangePasswordRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "PASSWORD_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangePassword(r.Context(), actor(r), id(r), req.Password)
	})
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
	req, err := platform.DecodeJSON[apidoc.ChangeStatusRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "STATUS_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangeStatus(r.Context(), actor(r), id(r), req.Status)
	})
}

// ChangeAdminRole documents administrator role reassignment.
//
// @Summary Change an administrator's role
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Administrator ID"
// @Param request body apidoc.ChangeRoleRequest true "Replacement role"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/role [patch]
func (h *Handler) ChangeAdminRole(w http.ResponseWriter, r *http.Request) {
	req, err := platform.DecodeJSON[apidoc.ChangeRoleRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "ROLE_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangeRole(r.Context(), actor(r), id(r), req.Role)
	})
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
func (h *Handler) ListApps(w http.ResponseWriter, r *http.Request) {
	page[domain.App](w, r, h.db.WithContext(r.Context()).Model(&domain.App{}), "created_at desc")
}

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
	req, err := platform.DecodeJSON[apidoc.CreateAppRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 201, "APP_CREATE_FAILED", func() (*domain.App, error) {
		return h.apps.CreateApp(r.Context(), actor(r), req.Name, req.Description, req.Environment)
	})
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
	req, err := platform.DecodeJSON[apidoc.UpdateAppRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "APP_UPDATE_FAILED", func() (*domain.App, error) {
		return h.apps.UpdateApp(r.Context(), actor(r), id(r), req.Name, req.Description, req.Environment)
	})
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
	req, err := platform.DecodeJSON[apidoc.ChangeStatusRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "APP_STATUS_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.apps.ChangeAppStatus(r.Context(), actor(r), id(r), req.Status)
	})
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
	page[domain.AppCredential](w, r, h.db.WithContext(r.Context()).Model(&domain.AppCredential{}).Where("app_id = ?", id(r)), "created_at desc")
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
	req, err := platform.DecodeJSON[apidoc.CreateCredentialRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	expires, err := parseExpiry(req.ExpiresAt)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 201, "APP_CREDENTIAL_CREATE_FAILED", func() (*services.CreatedCredential, error) {
		return h.apps.CreateCredential(r.Context(), actor(r), id(r), req.Name, req.Scopes, expires)
	})
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
	reply(w, 200, "APP_CREDENTIAL_REVOKE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.apps.RevokeCredential(r.Context(), actor(r), id(r), r.PathValue("credentialID"))
	})
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
	reply(w, 200, "APP_CREDENTIAL_ROTATE_FAILED", func() (*services.CreatedCredential, error) {
		return h.apps.RotateCredential(r.Context(), actor(r), id(r), r.PathValue("credentialID"))
	})
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
func (h *Handler) ListProviders(w http.ResponseWriter, r *http.Request) {
	page[domain.ProviderAccount](w, r, h.db.WithContext(r.Context()).Model(&domain.ProviderAccount{}), "created_at desc")
}

// ProviderRegistry writes the provider codes registered in this build, including
// any supplied by the embedding application, so that clients can offer them
// without knowing them in advance.
//
// @Summary List registered provider codes
// @Description Returns the provider codes accepted when creating a provider account.
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.ProviderRegistryResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/registry [get]
func (h *Handler) ProviderRegistry(w http.ResponseWriter, _ *http.Request) {
	platform.JSON(w, 200, apidoc.ProviderRegistry{Providers: h.providers.RegisteredProviders()})
}

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
	req, err := platform.DecodeJSON[apidoc.CreateProviderAccountRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 201, "PROVIDER_CREATE_FAILED", func() (*domain.ProviderAccount, error) {
		return h.providers.CreateAccount(r.Context(), actor(r), req.ProviderCode, req.Name, req.Environment, req.Countries, req.Config)
	})
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
	req, err := platform.DecodeJSON[apidoc.UpdateCountriesRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "COUNTRIES_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.UpdateCountries(r.Context(), actor(r), id(r), req.Countries)
	})
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
	req, err := platform.DecodeJSON[apidoc.UpdateProviderConfigRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "CONFIG_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.UpdateConfig(r.Context(), actor(r), id(r), req.Config)
	})
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
	reply(w, 200, "PROVIDER_ACTIVATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.Activate(r.Context(), actor(r), id(r))
	})
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
	reply(w, 200, "PROVIDER_DEACTIVATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.Deactivate(r.Context(), actor(r), id(r))
	})
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
	reply(w, 200, "PROVIDER_TEST_FAILED", func() (apidoc.OK, error) {
		err := h.runtime.TestProviderConfig(r.Context(), id(r))
		if err == nil {
			h.audit.RecordBestEffort(
				r.Context(),
				actor(r).ID,
				"admin",
				"provider.tested",
				"provider_account",
				id(r),
				nil,
				r.RemoteAddr,
				r.UserAgent(),
			)
		}
		return apidoc.OK{OK: true}, err
	})
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
func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	page[domain.PaymentRoute](w, r, h.db.WithContext(r.Context()).Model(&domain.PaymentRoute{}), "service_type asc, payment_method asc, priority asc")
}

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
	req, err := platform.DecodeJSON[apidoc.CreateRouteRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 201, "ROUTE_CREATE_FAILED", func() (*domain.PaymentRoute, error) {
		return h.routes.Create(r.Context(), actor(r), req.ServiceType, req.PaymentMethod, req.ProviderAccountID, req.Priority, req.Active)
	})
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
	req, err := platform.DecodeJSON[apidoc.UpdateRouteRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	reply(w, 200, "ROUTE_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.routes.Update(r.Context(), actor(r), id(r), req.Priority, req.Active)
	})
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

// page is a generic helper for paginated queries.
func page[T interface{}](w http.ResponseWriter, r *http.Request, query *gorm.DB, order string) {
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
