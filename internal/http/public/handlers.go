package public

import (
	"github.com/gofiber/fiber/v3"

	"errors"
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/dto"
	httpcommon "github.com/momobasehq/momobase/internal/http/common"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/service/payment"
	"github.com/momobasehq/momobase/internal/service/routing"
)

// Ping writes a plain 200 response for the liveness endpoint.
//
// @Summary Check liveness
// @Tags System
// @Success 200
// @Router /ping [get]
func Ping(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }

// Health writes the lightweight health payload.
//
// @Summary Check API health
// @Tags System
// @Produce json
// @Success 200 {object} apidoc.DocResponse
// @Router /healthz [get]
func Health(c fiber.Ctx) error {
	return platform.JSON(c, 200, map[string]bool{"ok": true})
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
func ClientToken(auth *identity.AppAuthService) fiber.Handler {
	return httpcommon.Token("client_credentials", func(c fiber.Ctx) (*identity.TokenResponse, error) {
		return auth.IssueClientToken(c, c.FormValue("client_id"), c.FormValue("client_secret"))
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
func AppRefreshToken(auth *identity.AppAuthService) fiber.Handler {
	return httpcommon.Token("refresh_token", func(c fiber.Ctx) (*identity.TokenResponse, error) {
		return auth.RefreshToken(c, c.FormValue("refresh_token"))
	})
}

// Handler serves authenticated client-facing payment endpoints.
type Handler struct {
	payments *payment.Orchestrator
	routes   *routing.Engine
	repos    *repository.UnitOfWork
}

// NewHandler constructs a public API handler from a payment orchestrator, the
// route engine backing method discovery, and a database connection.
func NewHandler(p *payment.Orchestrator, routes *routing.Engine, repos *repository.UnitOfWork) *Handler {
	return &Handler{payments: p, routes: routes, repos: repos}
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
func (h *Handler) ListPaymentMethods(c fiber.Ctx) error {
	methods, err := h.routes.AvailablePaymentMethods(
		c.Context(),
		strings.ToLower(strings.TrimSpace(c.Query("service_type"))),
		c.Query("country"),
	)
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	// Always an array, never null: a checkout screen iterating the response should
	// render "no methods available" rather than crash on a nil.
	return platform.JSON(c, fiber.StatusOK, map[string]any{"items": methods, "count": len(methods)})
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
// @Param request body dto.CreatePayment true "Collection request"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 503 {object} apidoc.ErrorResponse
// @Router /api/v1/collections [post]
func (h *Handler) CreateCollection(c fiber.Ctx) error {
	return h.create(c, domain.ServiceCollection)
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
// @Param request body dto.CreatePayment true "Disbursement request"
// @Success 201 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Failure 415 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Failure 503 {object} apidoc.ErrorResponse
// @Router /api/v1/disbursements [post]
func (h *Handler) CreateDisbursement(c fiber.Ctx) error {
	return h.create(c, domain.ServiceDisbursement)
}
func (h *Handler) create(c fiber.Ctx, service string) error {
	id := authmw.App(c)
	if id == nil {
		return platform.Error(c, 401, "UNAUTHORIZED", "missing app identity")
	}
	req, err := platform.DecodeJSON[dto.CreatePayment](c)
	if err != nil {
		return platform.Error(c, 400, "VALIDATION_ERROR", err.Error())
	}
	out, err := h.payments.Create(c.Context(), id.App.ID, service, c.Get("Idempotency-Key"), req)
	if errors.Is(err, routing.ErrNoRouteAvailable) {
		return platform.Error(c, 503, "ROUTE_UNAVAILABLE", "no active provider route is available")
	}
	if err != nil {
		return platform.Error(c, 400, "PAYMENT_ERROR", err.Error())
	}
	return platform.JSON(c, 201, out)
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
func (h *Handler) GetTransaction(c fiber.Ctx) error {
	return h.get(c, "id", c.Params("id"))
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
func (h *Handler) GetTransactionByReference(c fiber.Ctx) error {
	return h.get(c, "reference", c.Params("reference"))
}
func (h *Handler) get(c fiber.Ctx, field, value string) error {
	id := authmw.App(c)
	if id == nil {
		return platform.Error(c, 401, "UNAUTHORIZED", "missing app identity")
	}
	tx, err := h.payments.Get(c.Context(), id.App.ID, field, value)
	if err != nil {
		return platform.Error(c, 404, "NOT_FOUND", "transaction not found")
	}
	return platform.JSON(c, 200, tx)
}
