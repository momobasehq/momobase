package webhooks

import (
	"io"
	"net/http"

	"momobase/internal/platform"
	"momobase/internal/services"
)

type Handler struct{ service *services.WebhookService }

func NewHandler(s *services.WebhookService) *Handler { return &Handler{service: s} }
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
