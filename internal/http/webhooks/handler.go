package webhooks

import (
	"io"
	"net/http"

	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/webhook"
)

// Handler serves incoming provider webhook requests.
type Handler struct{ service *webhook.Service }

// NewHandler constructs a provider webhook handler from a webhook service.
func NewHandler(s *webhook.Service) *Handler { return &Handler{service: s} }

// ProviderWebhook reads an incoming provider webhook and delegates validation
// and processing to the webhook service.
//
// @Summary Receive a provider webhook
// @Description Verifies, deduplicates, and applies a provider-specific webhook payload.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param providerAccountID path string true "Provider account ID"
// @Param payload body object true "Provider-specific webhook payload"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 413 {object} apidoc.ErrorResponse
// @Failure 429 {object} apidoc.ErrorResponse
// @Router /webhooks/{providerAccountID} [post]
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
