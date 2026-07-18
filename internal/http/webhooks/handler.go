package webhooks

import (
	"io"
	"net/http"

	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

// Handler serves incoming provider webhook requests.
type Handler struct{ service *services.WebhookService }

// NewHandler constructs a provider webhook handler from a webhook service.
func NewHandler(s *services.WebhookService) *Handler { return &Handler{service: s} }

// ProviderWebhook reads an incoming provider webhook and delegates validation
// and processing to the webhook service.
func (h *Handler) ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		platform.Error(w, 400, "BAD_REQUEST", err.Error())
		return
	}
	headers := map[string]string{}
	for k := range r.Header {
		headers[k] = r.Header.Get(k)
	}
	if err := h.service.Handle(r.Context(), r.PathValue("providerAccountID"), payload, headers); err != nil {
		platform.Error(w, 400, "WEBHOOK_ERROR", err.Error())
		return
	}
	platform.JSON(w, 200, map[string]any{"ok": true})
}
