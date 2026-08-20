package webhooks

import (
	"github.com/gofiber/fiber/v3"

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
func (h *Handler) ProviderWebhook(c fiber.Ctx) error {
	// Body is the buffered request body; the provider secret and the payload hash are
	// both taken from it, so it must be read whole before anything is decided.
	payload := c.Body()
	headers := map[string]string{}
	for key, values := range c.GetReqHeaders() {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	if err := h.service.Handle(c.Context(), c.Params("providerAccountID"), payload, headers); err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "WEBHOOK_ERROR", err.Error())
	}
	return platform.JSON(c, 200, map[string]any{"ok": true})
}
