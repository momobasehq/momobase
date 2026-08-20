package admin

import (
	"github.com/gofiber/fiber/v3"

	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
)

// TransactionAnalytics writes a bucketed transaction series for the dashboard charts.
//
// @Summary Transaction analytics
// @Tags Admin - Analytics
// @Produce json
// @Security BearerAuth
// @Param from query string false "Inclusive RFC3339 start; defaults to 30 days ago"
// @Param to query string false "Exclusive RFC3339 end; defaults to now"
// @Param interval query string false "Bucket width" Enums(day, hour)
// @Param app_id query string false "Restrict to one application"
// @Param provider_account_id query string false "Restrict to one provider account"
// @Success 200 {object} apidoc.DocResponse
// @Failure 400 {object} apidoc.ErrorResponse
// @Failure 401 {object} apidoc.ErrorResponse
// @Failure 403 {object} apidoc.ErrorResponse
// @Router /api/admin/analytics/transactions [get]
func (h *Handler) TransactionAnalytics(c fiber.Ctx) error {
	from, err := optionalTime(c.Query("from"))
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "from must be an RFC3339 timestamp")
	}
	to, err := optionalTime(c.Query("to"))
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", "to must be an RFC3339 timestamp")
	}
	result, err := h.analytics.Transactions(c.Context(), identity.AnalyticsFilter{
		From:              from,
		To:                to,
		Interval:          c.Query("interval"),
		AppID:             strings.TrimSpace(c.Query("app_id")),
		ProviderAccountID: strings.TrimSpace(c.Query("provider_account_id")),
	})
	if err != nil {
		return platform.Error(c, fiber.StatusBadRequest, "BAD_REQUEST", err.Error())
	}
	return platform.JSON(c, fiber.StatusOK, result)
}

// optionalTime parses an RFC3339 timestamp, treating an absent value as unset rather
// than as an error, so a caller can supply one bound and let the other default.
func optionalTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
