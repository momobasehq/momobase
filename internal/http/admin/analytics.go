package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
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
func (h *Handler) TransactionAnalytics(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from, err := optionalTime(query.Get("from"))
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", "from must be an RFC3339 timestamp")
		return
	}
	to, err := optionalTime(query.Get("to"))
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", "to must be an RFC3339 timestamp")
		return
	}
	result, err := h.analytics.Transactions(r.Context(), services.AnalyticsFilter{
		From:              from,
		To:                to,
		Interval:          query.Get("interval"),
		AppID:             strings.TrimSpace(query.Get("app_id")),
		ProviderAccountID: strings.TrimSpace(query.Get("provider_account_id")),
	})
	if err != nil {
		platform.Error(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	platform.JSON(w, http.StatusOK, result)
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
