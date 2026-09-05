// Package extension demonstrates a compiled, app-scoped Momobase extension.
package extension

import (
	"context"
	"errors"
	"log/slog"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/hooks"
)

// Register rejects payments above maxAmount for appID and logs its committed
// transaction status changes. Call it after momobase.New and before Run or Serve.
func Register(instance *momobase.Instance, appID string, maxAmount int64) {
	instance.OnPaymentRequest().Bind(func(_ context.Context, event hooks.PaymentRequestEvent) error {
		if event.AppID == appID && event.Amount > maxAmount {
			return errors.New("payment exceeds the application limit")
		}
		return nil
	})
	instance.OnTransactionChanged().Bind(func(ctx context.Context, event hooks.TransactionChangedEvent) error {
		if event.AppID != appID {
			return nil
		}
		instance.Logger().InfoContext(
			ctx,
			"application transaction changed",
			slog.String("transaction_id", event.TransactionID),
			slog.String("status", event.Status),
			slog.String("source", event.Source),
		)
		return nil
	})
}
