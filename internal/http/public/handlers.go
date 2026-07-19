package public

import (
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	authmw "github.com/momobasehq/momobase/internal/http/middleware"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

// Handler serves authenticated client-facing payment endpoints.
type Handler struct {
	payments *services.PaymentOrchestrator
	db       *gorm.DB
}

// NewHandler constructs a public API handler from a payment orchestrator and
// database connection.
func NewHandler(p *services.PaymentOrchestrator, db *gorm.DB) *Handler {
	return &Handler{payments: p, db: db}
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
	if errors.Is(err, services.ErrNoRouteAvailable) {
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
