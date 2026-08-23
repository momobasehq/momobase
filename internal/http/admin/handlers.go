package admin

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	"github.com/momobasehq/momobase/internal/http/apidoc"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/service/provider"
	"github.com/momobasehq/momobase/internal/service/routing"
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
	repos     *repository.UnitOfWork
	auth      *identity.AdminAuthService
	users     *identity.AdminUserService
	providers *provider.AdminService
	routes    *routing.AdminService
	apps      *identity.AppService
	runtime   *provider.RuntimeManager
	audit     *audit.Service
	authz     *identity.AuthzService
	analytics *identity.AnalyticsService
	system    SystemInfo
}

type Response interface{}

// Deps carries the services an administrative handler needs.
//
// It is a struct rather than a positional argument list because these fields come
// from five packages and several share a shape: two adjacent *services pointers
// swapped by mistake would still compile, and the handler would quietly serve the
// wrong thing.
type Deps struct {
	// DB is the database handle used by read-only listing endpoints.
	Repos *repository.UnitOfWork
	// Auth issues and validates administrator sessions.
	Auth *identity.AdminAuthService
	// Users administers administrator accounts.
	Users *identity.AdminUserService
	// Providers administers provider accounts and their encrypted configuration.
	Providers *provider.AdminService
	// Routes administers payment routes.
	Routes *routing.AdminService
	// Apps administers applications and their credentials.
	Apps *identity.AppService
	// Runtime exposes loaded adapters for status and balance endpoints.
	Runtime *provider.RuntimeManager
	// Audit records administrative actions.
	Audit *audit.Service
	// Authz resolves and maintains roles and permissions.
	Authz *identity.AuthzService
	// Analytics answers transaction reporting queries.
	Analytics *identity.AnalyticsService
	// System describes the running build for the status endpoint.
	System SystemInfo
}

// NewHandler constructs an administrative HTTP handler from its services and
// system metadata.
func NewHandler(deps Deps) *Handler {
	return &Handler{
		repos:     deps.Repos,
		auth:      deps.Auth,
		users:     deps.Users,
		providers: deps.Providers,
		routes:    deps.Routes,
		apps:      deps.Apps,
		runtime:   deps.Runtime,
		audit:     deps.Audit,
		authz:     deps.Authz,
		analytics: deps.Analytics,
		system:    deps.System,
	}
}

// bind decodes a request body and lets the payload validate itself, which is the only
// validation an administrative handler does: everything past this point is a question
// about the database or the caller, and belongs to a service.
func bind[T any, P interface {
	*T
	dto.Payload
}](c fiber.Ctx) (P, error) {
	body, err := platform.DecodeJSON[T](c)
	if err != nil {
		return nil, err
	}
	payload := P(body)
	if err := dto.Check(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func actor(c fiber.Ctx) *domain.AdminUser { return authmw.AdminUser(c) }
func id(c fiber.Ctx) string               { return c.Params("id") }

// reply is a generic helper for handling HTTP responses.
func reply[T Response](c fiber.Ctx, status int, code string, run func() (T, error)) error {
	out, err := run()
	if err != nil {
		return platform.Error(c, 400, code, err.Error())
	}
	return platform.JSON(c, status, out)
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
func (h *Handler) Logout(c fiber.Ctx) error {
	return reply(c, 200, "LOGOUT_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.auth.LogoutBearer(c.Context(), authmw.BearerToken(c), c.IP(), c.Get(fiber.HeaderUserAgent))
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
func (h *Handler) ListTransactions(c fiber.Ctx) error {
	return page(c, h.repos.Transactions.Page)
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
func (h *Handler) ListAuditLogs(c fiber.Ctx) error {
	return page(c, h.repos.AuditLogs.Page)
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
func (h *Handler) ListProviderHealth(c fiber.Ctx) error {
	return page(c, h.repos.ProviderHealth.Page)
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
func (h *Handler) ListAdmins(c fiber.Ctx) error {
	return page(c, h.repos.AdminUsers.Page)
}

// CreateAdminUser documents administrator creation.
//
// @Summary Create an administrator
// @Tags Admin - Users
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateAdminRequest true "Administrator"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users [post]
func (h *Handler) CreateAdminUser(c fiber.Ctx) error {
	req, err := bind[dto.CreateAdminRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 201, "ADMIN_CREATE_FAILED", func() (*domain.AdminUser, error) {
		return h.users.Create(c.Context(), actor(c), req.Name, req.Email, req.Password, req.Role)
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
// @Param request body dto.ChangePasswordRequest true "New password"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/password [patch]
func (h *Handler) ChangeAdminPassword(c fiber.Ctx) error {
	req, err := bind[dto.ChangePasswordRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "PASSWORD_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangePassword(c.Context(), actor(c), id(c), req.Password)
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
// @Param request body dto.ChangeAdminStatusRequest true "Status" SchemaExample({"status":"active"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/status [patch]
func (h *Handler) ChangeAdminStatus(c fiber.Ctx) error {
	req, err := bind[dto.ChangeAdminStatusRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "STATUS_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangeStatus(c.Context(), actor(c), id(c), req.Status)
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
// @Param request body dto.ChangeRoleRequest true "Replacement role"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Router /api/admin/users/{id}/role [patch]
func (h *Handler) ChangeAdminRole(c fiber.Ctx) error {
	req, err := bind[dto.ChangeRoleRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "ROLE_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.users.ChangeRole(c.Context(), actor(c), id(c), req.Role)
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
func (h *Handler) ListApps(c fiber.Ctx) error {
	return page(c, h.repos.Apps.Page)
}

// CreateApp documents application creation.
//
// @Summary Create an application
// @Tags Admin - Applications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateAppRequest true "Application"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps [post]
func (h *Handler) CreateApp(c fiber.Ctx) error {
	req, err := bind[dto.CreateAppRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 201, "APP_CREATE_FAILED", func() (*domain.App, error) {
		return h.apps.CreateApp(c.Context(), actor(c), identity.CreateAppInput{
			Name:        req.Name,
			Description: req.Description,
			Environment: req.Environment,
			Currency:    req.Currency,
			Charges:     req.Charges,
		})
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
// @Param request body dto.UpdateAppRequest true "Application changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id} [patch]
func (h *Handler) UpdateApp(c fiber.Ctx) error {
	req, err := bind[dto.UpdateAppRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "APP_UPDATE_FAILED", func() (*domain.App, error) {
		return h.apps.UpdateApp(c.Context(), actor(c), id(c), identity.UpdateAppInput{
			Name:        req.Name,
			Description: req.Description,
			Environment: req.Environment,
			Currency:    req.Currency,
			Charges:     req.Charges,
		})
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
// @Param request body dto.ChangeAppStatusRequest true "Status" SchemaExample({"status":"disabled"})
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/status [patch]
func (h *Handler) ChangeAppStatus(c fiber.Ctx) error {
	req, err := bind[dto.ChangeAppStatusRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "APP_STATUS_CHANGE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.apps.ChangeAppStatus(c.Context(), actor(c), id(c), req.Status)
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
func (h *Handler) ListCredentials(c fiber.Ctx) error {
	appID := id(c)
	return page(c, func(ctx context.Context, number, size int) (repository.Page[domain.AppCredential], error) {
		return h.repos.AppCredentials.PageForApp(ctx, appID, number, size)
	})
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
// @Param request body dto.CreateCredentialRequest true "Credential"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id}/credentials [post]
func (h *Handler) CreateCredential(c fiber.Ctx) error {
	req, err := bind[dto.CreateCredentialRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	expires, err := parseExpiry(req.ExpiresAt)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 201, "APP_CREDENTIAL_CREATE_FAILED", func() (*identity.CreatedCredential, error) {
		return h.apps.CreateCredential(c.Context(), actor(c), id(c), req.Name, req.Scopes, expires)
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
func (h *Handler) RevokeCredential(c fiber.Ctx) error {
	return reply(c, 200, "APP_CREDENTIAL_REVOKE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.apps.RevokeCredential(c.Context(), actor(c), id(c), c.Params("credentialID"))
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
func (h *Handler) RotateCredential(c fiber.Ctx) error {
	return reply(c, 200, "APP_CREDENTIAL_ROTATE_FAILED", func() (*identity.CreatedCredential, error) {
		return h.apps.RotateCredential(c.Context(), actor(c), id(c), c.Params("credentialID"))
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
func (h *Handler) ListProviders(c fiber.Ctx) error {
	return page(c, h.repos.ProviderAccounts.Page)
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
func (h *Handler) ProviderRegistry(c fiber.Ctx) error {
	return platform.JSON(c, 200, apidoc.ProviderRegistry{Providers: h.providers.RegisteredProviders()})
}

// CreateProvider documents provider account creation.
//
// @Summary Create a provider account
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateProviderAccountRequest true "Provider account"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts [post]
func (h *Handler) CreateProvider(c fiber.Ctx) error {
	req, err := bind[dto.CreateProviderAccountRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 201, "PROVIDER_CREATE_FAILED", func() (*domain.ProviderAccount, error) {
		return h.providers.CreateAccount(c.Context(), actor(c), provider.CreateAccountInput{
			ProviderCode: req.ProviderCode,
			Name:         req.Name,
			Environment:  req.Environment,
			Country:      req.Country,
			Currency:     req.Currency,
			Charges:      req.Charges,
			Config:       req.Config,
		})
	})
}

// UpdateProviderSettings documents provider routing and fee updates.
//
// @Summary Update provider settings
// @Tags Admin - Providers
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param request body dto.UpdateProviderSettingsRequest true "Provider settings"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/settings [patch]
func (h *Handler) UpdateProviderSettings(c fiber.Ctx) error {
	req, err := bind[dto.UpdateProviderSettingsRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "SETTINGS_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.UpdateSettings(c.Context(), actor(c), id(c), provider.AccountSettings{
			Country: req.Country, Currency: req.Currency, Charges: req.Charges,
		})
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
// @Param request body dto.UpdateProviderConfigRequest true "Provider configuration"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/config [patch]
func (h *Handler) UpdateProviderConfig(c fiber.Ctx) error {
	req, err := bind[dto.UpdateProviderConfigRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "CONFIG_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.UpdateConfig(c.Context(), actor(c), id(c), req.Config)
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
func (h *Handler) ActivateProvider(c fiber.Ctx) error {
	return reply(c, 200, "PROVIDER_ACTIVATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.Activate(c.Context(), actor(c), id(c))
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
func (h *Handler) DeactivateProvider(c fiber.Ctx) error {
	return reply(c, 200, "PROVIDER_DEACTIVATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.providers.Deactivate(c.Context(), actor(c), id(c))
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
func (h *Handler) TestProvider(c fiber.Ctx) error {
	return reply(c, 200, "PROVIDER_TEST_FAILED", func() (apidoc.OK, error) {
		err := h.runtime.TestProviderConfig(c.Context(), id(c))
		if err == nil {
			h.audit.RecordBestEffort(
				c,
				actor(c).ID,
				"admin",
				"provider.tested",
				"provider_account",
				id(c),
				nil,
				c.IP(),
				c.Get(fiber.HeaderUserAgent),
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
func (h *Handler) ListRoutes(c fiber.Ctx) error {
	return page(c, h.repos.PaymentRoutes.Page)
}

// CreateRoute documents payment route creation.
//
// @Summary Create a payment route
// @Tags Admin - Routes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateRouteRequest true "Payment route"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes [post]
func (h *Handler) CreateRoute(c fiber.Ctx) error {
	req, err := bind[dto.CreateRouteRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 201, "ROUTE_CREATE_FAILED", func() (*domain.PaymentRoute, error) {
		return h.routes.Create(c.Context(), actor(c), req.ServiceType, req.PaymentMethod, req.ProviderAccountID, req.Priority, req.Active)
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
// @Param request body dto.UpdateRouteRequest true "Route changes"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/routes/{id} [patch]
func (h *Handler) UpdateRoute(c fiber.Ctx) error {
	req, err := bind[dto.UpdateRouteRequest](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	return reply(c, 200, "ROUTE_UPDATE_FAILED", func() (apidoc.OK, error) {
		return apidoc.OK{OK: true}, h.routes.Update(c.Context(), actor(c), id(c), req.Priority, req.Active)
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

// page answers a listing from a repository's own paged reader. The bounds come from
// the query string; the query itself stays inside the repository.
func page[T any](c fiber.Ctx, read func(context.Context, int, int) (repository.Page[T], error)) error {
	number, size := platform.Pagination(c)
	result, err := read(c.Context(), number, size)
	if err != nil {
		return platform.Error(c, 500, "SERVER_ERROR", err.Error())
	}
	return platform.JSON(c, 200, platform.PageData[T]{
		Page:  number,
		Total: int(result.Total),
		Items: result.Items,
		Count: len(result.Items),
	})
}
