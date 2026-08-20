package momobase

import (
	"context"
	"net/http"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// The provider contract. These are aliases of the internal definitions, so a
// type implementing momobase.PaymentProvider satisfies the engine directly.
type (
	// PaymentProvider defines the operations implemented by a payment provider.
	// Implement this interface to add a provider, then supply it to New with
	// WithProvider.
	PaymentProvider = providers.PaymentProvider

	// ProviderFactory constructs a payment provider using the supplied logger. The
	// logger is never nil and is already tagged with the provider code.
	ProviderFactory = providers.Factory

	// Capability identifies a payment service supported by a provider.
	Capability = providers.Capability

	// ProviderConfig contains the provider-specific values used during initialization.
	// It holds the decrypted configuration recorded for a provider account.
	ProviderConfig = providers.ProviderConfig

	// PaymentRequest contains the normalized details of a provider payment operation.
	PaymentRequest = providers.PaymentRequest

	// ProviderPaymentResponse describes a payment request accepted by a provider.
	ProviderPaymentResponse = providers.ProviderPaymentResponse

	// ProviderTransactionStatus describes the current state of a provider transaction.
	ProviderTransactionStatus = providers.ProviderTransactionStatus

	// ProviderBalance contains a provider account's balances in minor currency units.
	ProviderBalance = providers.ProviderBalance

	// ProviderWebhookEvent contains normalized data extracted from a provider webhook.
	ProviderWebhookEvent = providers.ProviderWebhookEvent

	// RequestValidator is the optional interface a provider implements to validate
	// and normalize a payment request before Momobase persists a transaction.
	// Account validation belongs here: the engine treats an account as opaque.
	RequestValidator = providers.RequestValidator
)

// Service types and transaction statuses. A provider must report one of the Tx
// values as the normalized status of an operation.
const (
	// ServiceCollection identifies an incoming payment collection.
	ServiceCollection = domain.ServiceCollection
	// ServiceDisbursement identifies an outgoing payment disbursement.
	ServiceDisbursement = domain.ServiceDisbursement

	// TxPending indicates that a transaction is waiting to be processed.
	TxPending = domain.TxPending
	// TxProcessing indicates that transaction processing has started.
	TxProcessing = domain.TxProcessing
	// TxSucceeded indicates that a transaction completed successfully.
	TxSucceeded = domain.TxSucceeded
	// TxFailed indicates that a transaction failed permanently.
	TxFailed = domain.TxFailed
	// TxUnknown indicates that the provider outcome is not yet known.
	TxUnknown = domain.TxUnknown
	// TxCancelled indicates that a transaction was cancelled.
	TxCancelled = domain.TxCancelled
	// TxExpired indicates that a transaction expired before completion.
	TxExpired = domain.TxExpired
)

// ErrCircuitOpen indicates that a provider request was rejected by an open circuit breaker.
var ErrCircuitOpen = providers.ErrCircuitOpen

// Supports reports whether caps contains the requested service.
func Supports(caps []Capability, service string) bool {
	return providers.Supports(caps, service)
}

// DoJSON sends an HTTP request with an optional JSON body and decodes a successful
// JSON response into out. Non-2xx responses are returned as redacted errors.
func DoJSON(
	ctx context.Context,
	client *http.Client,
	method string,
	url string,
	headers map[string]string,
	in any,
	out any,
) error {
	return providers.DoJSON(ctx, client, method, url, headers, in, out)
}

// Redact removes credential-like values from text before it is logged or returned.
func Redact(value string) string {
	return providers.Redact(value)
}

// ConfigString returns a trimmed textual representation of a configuration value.
func ConfigString(c ProviderConfig, key string) string {
	return utils.String(c, key)
}

// ConfigBool reports whether a configuration value is "true", case-insensitively, or "1".
func ConfigBool(c ProviderConfig, key string) bool {
	return utils.Bool(c, key)
}

// ConfigInt converts a configuration value to an integer, returning zero when invalid.
func ConfigInt(c ProviderConfig, key string) int {
	return utils.Int(c, key)
}

// ConfigPath returns the textual value at a dot-separated path through nested maps.
func ConfigPath(values map[string]any, path string) string {
	return utils.Path(values, path)
}

// First returns the first nonblank value after trimming surrounding whitespace.
func First(values ...string) string {
	return utils.First(values...)
}

// Slash ensures value begins with a forward slash, for joining configured API paths.
func Slash(value string) string {
	return utils.Slash(value)
}

// FirstError returns primary when it is non-nil and fallback otherwise.
func FirstError(primary, fallback error) error {
	return utils.FirstError(primary, fallback)
}

// PaymentStatus maps a provider status string onto a normalized Tx status.
func PaymentStatus(value string) string {
	return providers.PaymentStatus(value)
}

// ParseAmountToMinor converts a decimal amount string into minor currency units.
func ParseAmountToMinor(raw, currency string) (int64, error) {
	return providers.ParseAmountToMinor(raw, currency)
}

// FormatAmountMinor renders an amount in minor currency units as a decimal string.
func FormatAmountMinor(amount int64, currency string) string {
	return providers.FormatAmountMinor(amount, currency)
}

// OptionalAmount converts a decimal amount string into minor currency units,
// returning nil when raw is blank.
func OptionalAmount(raw, currency string) (*int64, error) {
	return providers.OptionalAmount(raw, currency)
}

// RandomRef returns a random reference with the supplied prefix.
func RandomRef(prefix string) string {
	return providers.RandomRef(prefix)
}

// UUID returns a random UUID string.
func UUID() string {
	return providers.UUID()
}
