# Add payment hooks

Hooks add synchronous, in-process behavior to a Momobase instance compiled as a Go application. Use them to reject a new payment under application-specific policy or observe a committed transaction status change.

Register hooks after `momobase.New()` and before `Run()` or `Serve()`:

```go
package extension

import (
	"context"
	"errors"
	"log/slog"

	"github.com/momobasehq/momobase"
	"github.com/momobasehq/momobase/hooks"
)

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
		instance.Logger().InfoContext(ctx, "transaction changed",
			slog.String("transaction_id", event.TransactionID),
			slog.String("status", event.Status),
			slog.String("source", event.Source),
		)
		return nil
	})
}
```

The complete example lives in [`examples/extension`](https://github.com/momobasehq/momobase/tree/main/examples/extension).

## Reject a payment request

`OnPaymentRequest()` runs after normalization and idempotency replay detection, but before routing and persistence. Handlers run in registration order and the first error stops the chain.

An error rejects the request with the stable API code `PAYMENT_REJECTED`. Momobase logs the handler error but does not return its text to the caller.

The event includes account and party data. Treat these fields as sensitive and do not copy them into logs or returned errors.

## Observe a transaction change

`OnTransactionChanged()` runs after a status change commits. `Source` is `request`, `webhook`, or `reconciliation`. An observer error is logged, remaining observers still run, and the committed transaction is not rolled back.

Use this hook for telemetry or best-effort side effects. It is not a durable message-delivery mechanism. Use a transactional outbox when another system must eventually receive every change.

## Remove a handler

`Bind()` returns an idempotent function that unregisters the handler:

```go
unbind := instance.OnTransactionChanged().Bind(handler)
defer unbind()
```

Binding and removal are safe during concurrent invocation. A handler already included in the current invocation snapshot may still finish.
