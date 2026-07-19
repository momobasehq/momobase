package admin

import (
	"net/http"

	httpcommon "github.com/momobasehq/momobase/internal/http/common"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
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
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) { platform.JSON(w, 200, actor(r)) }

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
func (h *Handler) Token(w http.ResponseWriter, r *http.Request) {
	httpcommon.Token("password", func(r *http.Request) (*services.TokenResponse, error) {
		return h.auth.IssuePasswordToken(r.Context(), r.Form.Get("username"), r.Form.Get("password"), r.RemoteAddr, r.UserAgent())
	})(w, r)
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
func (h *Handler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	httpcommon.Token("refresh_token", func(r *http.Request) (*services.TokenResponse, error) {
		return h.auth.RefreshToken(r.Context(), r.Form.Get("refresh_token"), r.RemoteAddr, r.UserAgent())
	})(w, r)
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
func (h *Handler) ProviderBalance(w http.ResponseWriter, r *http.Request) {
	out, err := h.runtime.QueryBalance(r.Context(), id(r), r.URL.Query().Get("country"))
	if err != nil {
		platform.Error(w, 400, "BALANCE_QUERY_FAILED", err.Error())
		return
	}
	h.audit.RecordBestEffort(r.Context(), actor(r).ID, "admin", "balance.queried", "provider_account", id(r), nil, r.RemoteAddr, r.UserAgent())
	platform.JSON(w, 200, out)
}
