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

type Handler struct {
	payments *services.PaymentOrchestrator
	db       *gorm.DB
}

func NewHandler(p *services.PaymentOrchestrator, db *gorm.DB) *Handler {
	return &Handler{payments: p, db: db}
}
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	h.create(w, r, domain.ServiceCollection)
}
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
func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, "id", r.PathValue("id"))
}
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
