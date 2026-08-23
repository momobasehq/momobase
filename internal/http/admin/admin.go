package admin

import (
	"github.com/gofiber/fiber/v3"

	httpcommon "github.com/momobasehq/momobase/internal/http/common"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
)

// Me writes the authenticated administrator stored in the request context.
//
// @Summary Get the current administrator
// @Tags Admin - Authentication
// @Produce json
// @Security BearerAuth
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/me [get]
func (h *Handler) Me(c fiber.Ctx) error { return platform.JSON(c, 200, actor(c)) }

// Token documents administrator login.
//
// @Summary Issue administrator tokens
// @Description The grant_type field may be omitted for compatibility with the admin login form.
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string false "OAuth grant type" Enums(password)
// @Param username formData string true "Administrator email"
// @Param password formData string true "Administrator password"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/token [post]
// @Router /api/admin/login [post]
func (h *Handler) Token(c fiber.Ctx) error {
	return httpcommon.Token("password", func(c fiber.Ctx) (*identity.TokenResponse, error) {
		return h.auth.IssuePasswordToken(c.Context(), c.FormValue("username"), c.FormValue("password"), c.IP(), c.Get(fiber.HeaderUserAgent))
	})(c)
}

// RefreshToken documents administrator token refresh.
//
// @Summary Refresh administrator tokens
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(refresh_token)
// @Param refresh_token formData string true "Administrator refresh token"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/token/refresh [post]
func (h *Handler) RefreshToken(c fiber.Ctx) error {
	return httpcommon.Token("refresh_token", func(c fiber.Ctx) (*identity.TokenResponse, error) {
		return h.auth.RefreshToken(c.Context(), c.FormValue("refresh_token"), c.IP(), c.Get(fiber.HeaderUserAgent))
	})(c)
}

// GetApp writes the application identified by the request path or a not-found
// response when no matching application exists.
//
// @Summary Get an application
// @Tags Admin - Applications
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/apps/{id} [get]
func (h *Handler) GetApp(c fiber.Ctx) error {
	app, err := h.apps.GetApp(c.Context(), id(c))
	if err != nil {
		return platform.Error(c, 404, "NOT_FOUND", "app not found")
	}
	return platform.JSON(c, 200, app)
}

// ActiveProviderBalances queries balances for active provider runtimes and
// writes them as a paginated response.
//
// @Summary Query all active provider balances
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" minimum(1)
// @Param per_page query int false "Items per page" minimum(1) maximum(100)
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/balances/providers [get]
func (h *Handler) ActiveProviderBalances(c fiber.Ctx) error {
	items, err := h.runtime.QueryActiveBalances(c.Context())
	if err != nil {
		return platform.Error(c, 400, "BALANCE_QUERY_FAILED", err.Error())
	}
	h.audit.RecordBestEffort(
		c,
		actor(c).ID,
		"admin",
		"balances.active_providers_queried",
		"provider_account",
		"all_active",
		nil,
		c.IP(),
		c.Get(fiber.HeaderUserAgent),
	)
	page, size := platform.Pagination(c)
	return platform.JSON(c, 200, platform.PaginateSlice(items, page, size))
}

// ProviderBalance queries and writes the balance for the provider account and its
// configured country. The optional query value must match when supplied.
//
// @Summary Query a provider balance
// @Tags Admin - Providers
// @Produce json
// @Security BearerAuth
// @Param id path string true "Provider account ID"
// @Param country query string false "ISO 3166-1 alpha-2 country code"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/admin/providers/accounts/{id}/balance [get]
func (h *Handler) ProviderBalance(c fiber.Ctx) error {
	out, err := h.runtime.QueryBalance(c.Context(), id(c), c.Query("country"))
	if err != nil {
		return platform.Error(c, 400, "BALANCE_QUERY_FAILED", err.Error())
	}
	h.audit.RecordBestEffort(c.Context(), actor(c).ID, "admin", "balance.queried", "provider_account", id(c), nil, c.IP(), c.Get(fiber.HeaderUserAgent))
	return platform.JSON(c, 200, out)
}
