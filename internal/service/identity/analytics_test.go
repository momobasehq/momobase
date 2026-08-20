package identity_test

import (
	"context"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/testsupport"
)

// seedTransaction inserts a transaction at an explicit time, since the buckets are
// keyed on created_at and BaseModel would otherwise stamp them all as now.
func seedTransaction(t *testing.T, s *testsupport.Stack, at time.Time, service, status, currency string, amount int64, appID, accountID string) {
	t.Helper()
	tx := domain.Transaction{
		BaseModel:                 domain.BaseModel{ID: platform.NewID("txn")},
		AppID:                     appID,
		ServiceType:               service,
		PaymentMethod:             testsupport.Method,
		Amount:                    amount,
		Currency:                  currency,
		Reference:                 platform.NewID("ref"),
		IdempotencyKey:            platform.NewID("idem"),
		Status:                    status,
		SelectedProviderAccountID: accountID,
	}
	testsupport.NoError(s.DB.Create(&tx).Error)
	// Written after creation: GORM populates CreatedAt on insert, so the backdating has
	// to overwrite it rather than be supplied alongside.
	testsupport.NoError(s.DB.Model(&domain.Transaction{}).Where("id = ?", tx.ID).Update("created_at", at).Error)
}

func TestTransactionAnalytics(t *testing.T) {
	s := testsupport.New(t)
	ctx := context.Background()
	// Midnight-aligned bounds: day buckets are truncated to a day boundary, so an
	// arbitrary start time would widen the range by a partial day and make the expected
	// bucket count depend on the hour the test runs.
	to := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	from := to.AddDate(0, 0, -3)
	day := func(offset int) time.Time { return from.Add(time.Duration(offset)*24*time.Hour + 2*time.Hour) }

	seedTransaction(t, s, day(0), domain.ServiceCollection, domain.TxSucceeded, "UGX", 5000, "app-1", "pacc-1")
	seedTransaction(t, s, day(0), domain.ServiceCollection, domain.TxFailed, "UGX", 2000, "app-1", "pacc-1")
	seedTransaction(t, s, day(1), domain.ServiceDisbursement, domain.TxSucceeded, "UGX", 1000, "app-2", "pacc-2")
	seedTransaction(t, s, day(2), domain.ServiceCollection, domain.TxSucceeded, "USD", 700, "app-1", "pacc-2")

	t.Run("buckets every period, including quiet ones", func(t *testing.T) {
		result := testsupport.Must(s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to}))
		if len(result.Buckets) != 3 {
			t.Fatalf("buckets = %d, want one per day in the range", len(result.Buckets))
		}
		if result.Total != 4 {
			t.Errorf("total = %d, want 4", result.Total)
		}
		if result.ByService.Collection != 3 || result.ByService.Disbursement != 1 {
			t.Errorf("by service = %+v, want 3 collections and 1 disbursement", result.ByService)
		}
		first := result.Buckets[0]
		if first.Total != 2 || first.Succeeded != 1 || first.Failed != 1 {
			t.Errorf("first bucket = %+v, want 2 total, 1 succeeded, 1 failed", first)
		}
	})

	// Summing amounts across currencies would produce a number that means nothing, so
	// volume stays split and is never totalled.
	t.Run("volume is reported per currency", func(t *testing.T) {
		result := testsupport.Must(s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to}))
		if len(result.Volume) != 2 {
			t.Fatalf("volume = %+v, want one entry per currency", result.Volume)
		}
		if result.Volume[0].Currency != "UGX" || result.Volume[0].Amount != 8000 {
			t.Errorf("UGX volume = %+v, want 8000", result.Volume[0])
		}
		if result.Volume[1].Currency != "USD" || result.Volume[1].Amount != 700 {
			t.Errorf("USD volume = %+v, want 700", result.Volume[1])
		}
	})

	t.Run("filters by app and by provider account", func(t *testing.T) {
		byApp := testsupport.Must(s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to, AppID: "app-1"}))
		if byApp.Total != 3 {
			t.Errorf("app-1 total = %d, want 3", byApp.Total)
		}
		byProvider := testsupport.Must(s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to, ProviderAccountID: "pacc-2"}))
		if byProvider.Total != 2 {
			t.Errorf("pacc-2 total = %d, want 2", byProvider.Total)
		}
		both := testsupport.Must(
			s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to, AppID: "app-1", ProviderAccountID: "pacc-2"}),
		)
		if both.Total != 1 {
			t.Errorf("app-1 + pacc-2 total = %d, want 1", both.Total)
		}
	})

	t.Run("hour buckets are finer than day buckets", func(t *testing.T) {
		result := testsupport.Must(s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: to.Add(-6 * time.Hour), To: to, Interval: "hour"}))
		if len(result.Buckets) != 6 {
			t.Errorf("hour buckets = %d, want 6", len(result.Buckets))
		}
	})

	// Refusing beats truncating: a capped series would render as a chart that quietly
	// omits part of its own range.
	t.Run("rejects a range too wide to render", func(t *testing.T) {
		if _, err := s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: to.AddDate(-3, 0, 0), To: to, Interval: "hour"}); err == nil {
			t.Error("Transactions() accepted a range beyond the bucket cap")
		}
	})

	t.Run("rejects an inverted range and an unknown interval", func(t *testing.T) {
		if _, err := s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: to, To: from}); err == nil {
			t.Error("Transactions() accepted an inverted range")
		}
		if _, err := s.Analytics.Transactions(ctx, identity.AnalyticsFilter{From: from, To: to, Interval: "week"}); err == nil {
			t.Error("Transactions() accepted an unknown interval")
		}
	})
}
