package public

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	httpcommon "github.com/momobasehq/momobase/internal/http/common"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/routing"
	"github.com/momobasehq/momobase/internal/services"
)

// Ping writes a plain 200 response for the liveness endpoint.
//
// @Summary Check liveness
// @Tags System
// @Success 200
// @Router /ping [get]
func Ping(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// Health writes the lightweight health payload.
//
// @Summary Check API health
// @Tags System
// @Produce json
// @Success 200 {object} apidoc.DocResponse
// @Router /healthz [get]
func Health(w http.ResponseWriter, _ *http.Request) {
	platform.JSON(w, 200, map[string]bool{"ok": true})
}

// ClientToken documents application token issuance.
//
// @Summary Issue application tokens
// @Description Validates an active client ID and secret and returns an access/refresh token pair.
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(client_credentials)
// @Param client_id formData string true "Application client ID"
// @Param client_secret formData string true "Application client secret"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/token [post]
func ClientToken(auth *services.AppAuthService) http.HandlerFunc {
	return httpcommon.Token("client_credentials", func(r *http.Request) (*services.TokenResponse, error) {
		return auth.IssueClientToken(r.Context(), r.Form.Get("client_id"), r.Form.Get("client_secret"))
	})
}

// AppRefreshToken documents application token refresh.
//
// @Summary Refresh application tokens
// @Tags Authentication
// @Accept x-www-form-urlencoded
// @Produce json
// @Param grant_type formData string true "OAuth grant type" Enums(refresh_token)
// @Param refresh_token formData string true "Application refresh token"
// @Success 200 {object} apidoc.TokenResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/token/refresh [post]
func AppRefreshToken(auth *services.AppAuthService) http.HandlerFunc {
	return httpcommon.Token("refresh_token", func(r *http.Request) (*services.TokenResponse, error) {
		return auth.RefreshToken(r.Context(), r.Form.Get("refresh_token"))
	})
}

// Handler serves authenticated client-facing payment endpoints.
type Handler struct {
	payments *services.PaymentOrchestrator
	routes   *routing.Engine
	db       *gorm.DB
}

// NewHandler constructs a public API handler from a payment orchestrator, the
// route engine backing method discovery, and a database connection.
func NewHandler(p *services.PaymentOrchestrator, routes *routing.Engine, db *gorm.DB) *Handler {
	return &Handler{payments: p, routes: routes, db: db}
}

// ListPaymentMethods writes the payment methods this deployment can currently
// serve, which is what a checkout screen offers before collecting any details.
//
// @Summary List available payment methods
// @Tags Payments
// @Produce json
// @Security BearerAuth
// @Param service_type query string false "Filter by collection or disbursement"
// @Param country query string false "ISO 3166-1 alpha-2 transaction country"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/payment-methods [get]
func (h *Handler) ListPaymentMethods(w http.ResponseWriter, r *http.Request) {
	methods, err := h.routes.AvailablePaymentMethods(
		r.Context(),
		strings.ToLower(strings.TrimSpace(r.URL.Query().Get("service_type"))),
		r.URL.Query().Get("country"),
	)
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	// Always an array, never null: a checkout screen iterating the response should
	// render "no methods available" rather than crash on a nil.
	platform.JSON(w, http.StatusOK, map[string]any{"items": methods, "count": len(methods)})
}

// CreateCollection validates and creates a collection transaction for the
// authenticated application.
//
// @Summary Create a collection
// @Description Creates and executes an idempotent mobile-money collection.
// @Tags Payments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param Idempotency-Key header string true "Unique idempotency key"
// @Param request body services.CreatePaymentRequest true "Collection request"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 503 {object} apidoc.ErrorResponse
// @Router /api/v1/collections [post]
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, domain.ServiceCollection)
}

// CreateDisbursement validates and creates a disbursement transaction for the
// authenticated application.
//
// @Summary Create a disbursement
// @Description Creates and executes an idempotent mobile-money disbursement.
// @Tags Payments
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param Idempotency-Key header string true "Unique idempotency key"
// @Param request body services.CreatePaymentRequest true "Disbursement request"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 503 {object} apidoc.ErrorResponse
// @Router /api/v1/disbursements [post]
func (h *Handler) CreateDisbursement(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, domain.ServiceDisbursement)
}
func (h *Handler) create(w http.ResponseWriter, r *http.Request, service string) {
	id := authmw.App(r)
	if id == nil {
		platform.Error(w, 401, "UNAUTHORIZED", "missing app identity")
		return
	}
	req, err := platform.DecodeJSON[services.CreatePaymentRequest](r)
	if err != nil {
		platform.Error(w, 400, "VALIDATION_ERROR", err.Error())
		return
	}
	out, err := h.payments.Create(r.Context(), id.App.ID, service, r.Header.Get("Idempotency-Key"), req)
	if errors.Is(err, routing.ErrNoRouteAvailable) {
		platform.Error(w, 503, "ROUTE_UNAVAILABLE", "no active provider route is available")
		return
	}
	if err != nil {
		platform.Error(w, 400, "PAYMENT_ERROR", err.Error())
		return
	}
	platform.JSON(w, 201, out)
}

// GetTransaction writes the transaction identified by the request path when it
// belongs to the authenticated application.
//
// @Summary Get a transaction
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param id path string true "Transaction ID"
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/transactions/{id} [get]
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, "id", r.PathValue("id"))
}

// GetTransactionByReference writes the transaction identified by the request
// reference when it belongs to the authenticated application.
//
// @Summary Get a transaction by reference
// @Tags Transactions
// @Produce json
// @Security BearerAuth
// @Param reference path string true "Application transaction reference"
// @Success 200 {object} apidoc.DocResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 404 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /api/v1/transactions/by-reference/{reference} [get]
func (h *Handler) GetTransactionByReference(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, "reference", r.PathValue("reference"))
}
func (h *Handler) get(w http.ResponseWriter, r *http.Request, field, value string) {
	id := authmw.App(r)
	if id == nil {
		platform.Error(w, 401, "UNAUTHORIZED", "missing app identity")
		return
	}
	var tx domain.Transaction
	if h.db.WithContext(r.Context()).Where("app_id = ? AND "+field+" = ?", id.App.ID, value).First(&tx).Error != nil {
		platform.Error(w, 404, "NOT_FOUND", "transaction not found")
		return
	}
	platform.JSON(w, 200, &tx)
}
