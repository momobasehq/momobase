package hooks

import (
	"context"
	"errors"
	"log/slog"
)

const (
	// TransactionSourceRequest identifies the initial payment request path.
	TransactionSourceRequest = "request"
	// TransactionSourceWebhook identifies a verified provider webhook path.
	TransactionSourceWebhook = "webhook"
	// TransactionSourceReconciliation identifies a provider polling path.
	TransactionSourceReconciliation = "reconciliation"
)

// ErrPaymentRejected reports that an extension rejected a new payment request.
// The handler's original error is logged but is not returned across the API boundary.
var ErrPaymentRejected = errors.New("payment request rejected by extension")

// PaymentRequestEvent is the normalized, immutable-by-contract snapshot presented
// before a new payment is routed or persisted. Idempotent replays do not trigger it.
// Account and party fields are customer data and must not be copied into errors or logs.
type PaymentRequestEvent struct {
	// AppID identifies the application making the request.
	AppID string
	// ServiceType identifies a collection or disbursement.
	ServiceType string
	// PaymentMethod identifies the requested payment rail.
	PaymentMethod string
	// Amount is the payment amount in currency minor units.
	Amount int64
	// Currency is the normalized three-letter currency code.
	Currency string
	// Country is the normalized ISO 3166-1 alpha-2 transaction country.
	Country string
	// Reference is the application's business reference.
	Reference string
	// Account identifies the payer or payee account.
	Account string
	// Scheme identifies the optional provider-specific account scheme.
	Scheme string
	// Description is the caller-supplied payment narrative.
	Description string
	// PartyName is the collection customer or disbursement recipient name.
	PartyName string
	// PartyEmail is the collection customer or disbursement recipient email.
	PartyEmail string
}

// TransactionChangedEvent is a persisted transaction status snapshot. It contains
// no provider credentials, raw responses, webhook bodies, or customer account data.
type TransactionChangedEvent struct {
	// Source identifies the request, webhook, or reconciliation path.
	Source string
	// AppID identifies the application that owns the transaction.
	AppID string
	// TransactionID is Momobase's transaction identifier.
	TransactionID string
	// Reference is the application's business reference.
	Reference string
	// ServiceType identifies a collection or disbursement.
	ServiceType string
	// PaymentMethod identifies the transaction's payment rail.
	PaymentMethod string
	// Amount is the payment amount in currency minor units.
	Amount int64
	// Currency is the normalized three-letter currency code.
	Currency string
	// Country is the normalized ISO 3166-1 alpha-2 transaction country.
	Country string
	// PreviousStatus is the status before the persisted change.
	PreviousStatus string
	// Status is the status after the persisted change.
	Status string
	// ProviderAccountID identifies the selected provider account.
	ProviderAccountID string
	// ProviderReference identifies the operation in the provider's system.
	ProviderReference string
}

// Registry owns the extension hooks shared by one Momobase instance.
type Registry struct {
	logger             *slog.Logger
	paymentRequest     Hook[PaymentRequestEvent]
	transactionChanged Hook[TransactionChangedEvent]
}

// NewRegistry returns an empty registry. A nil logger discards hook diagnostics.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Registry{logger: logger}
}

// OnPaymentRequest returns the blocking hook for normalized new payment requests.
func (r *Registry) OnPaymentRequest() *Hook[PaymentRequestEvent] {
	return &r.paymentRequest
}

// OnTransactionChanged returns the post-commit transaction status hook. Momobase
// continues invoking its remaining handlers when one returns an error.
func (r *Registry) OnTransactionChanged() *Hook[TransactionChangedEvent] {
	return &r.transactionChanged
}

// RunPaymentRequest invokes payment-request handlers and translates any extension
// failure into the stable rejection returned by the payment service.
func (r *Registry) RunPaymentRequest(ctx context.Context, event PaymentRequestEvent) error {
	if err := r.paymentRequest.Trigger(ctx, event); err != nil {
		r.logger.WarnContext(
			ctx,
			"payment request rejected by extension",
			slog.String("app_id", event.AppID),
			slog.String("error", err.Error()),
		)
		return ErrPaymentRejected
	}
	return nil
}

// NotifyTransactionChanged invokes post-commit handlers. Their errors are logged and
// deliberately not returned because the transaction change is already durable.
func (r *Registry) NotifyTransactionChanged(ctx context.Context, event TransactionChangedEvent) {
	if err := r.transactionChanged.triggerAll(ctx, event); err != nil {
		r.logger.ErrorContext(
			ctx,
			"transaction change extension failed",
			slog.String("app_id", event.AppID),
			slog.String("transaction_id", event.TransactionID),
			slog.String("source", event.Source),
			slog.String("error", err.Error()),
		)
	}
}
