package providers

import (
	"context"
	"errors"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
)

// PaymentMethod identifies one payment rail supported by Momobase.
type PaymentMethod = domain.PaymentMethod

// Payment methods accepted by routes and payment requests.
const (
	PaymentMethodMomo         = domain.PaymentMethodMomo
	PaymentMethodCard         = domain.PaymentMethodCard
	PaymentMethodBankTransfer = domain.PaymentMethodBankTransfer
	PaymentMethodWallet       = domain.PaymentMethodWallet
)

// PaymentMethods returns every supported payment method.
func PaymentMethods() []PaymentMethod { return domain.PaymentMethods() }

// ValidPaymentMethod reports whether method is supported by Momobase.
func ValidPaymentMethod(method PaymentMethod) bool { return domain.ValidPaymentMethod(method) }

// Capability identifies a payment service and method supported by a provider.
type Capability struct {
	// ServiceType identifies the payment service, such as collection or disbursement.
	ServiceType string `json:"service_type"`
	// PaymentMethod identifies the payment rail served by this capability.
	PaymentMethod PaymentMethod `json:"payment_method"`
}

// ProviderConfig contains the provider-specific flat values used during initialization.
// Momobase adds the provider account's authoritative "environment" value before
// calling Init, overriding any value stored in the encrypted provider config.
type ProviderConfig map[string]any

// ConfigString returns a trimmed textual representation of a configuration value.
func ConfigString(c ProviderConfig, key string) string { return utils.String(c, key) }

// ConfigBool reports whether a configuration value is "true", case-insensitively, or "1".
func ConfigBool(c ProviderConfig, key string) bool { return utils.Bool(c, key) }

// ConfigInt converts a configuration value to an integer, returning zero when invalid.
func ConfigInt(c ProviderConfig, key string) int { return utils.Int(c, key) }

// ConfigPath returns the textual value at a dot-separated path through nested maps.
func ConfigPath(values map[string]any, path string) string { return utils.Path(values, path) }

// First returns the first nonblank value after trimming surrounding whitespace.
func First(values ...string) string {
	return utils.First(values...)
}

// Service types and transaction statuses reported by providers.
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

// PaymentRequest contains the normalized details of a provider payment operation.
type PaymentRequest struct {
	// TransactionID identifies the Momobase transaction associated with the request.
	TransactionID string
	// PaymentMethod identifies the payment rail the route selected for this request.
	// A provider that serves more than one rail dispatches on it.
	PaymentMethod PaymentMethod
	// Currency is the ISO currency code for the payment amount.
	Currency string
	// Country is the ISO 3166-1 alpha-2 country for the payment, or empty when the
	// request carries none.
	Country string
	// Reference is the external reference supplied to the provider.
	Reference string
	// Account identifies the payer or payee account. Its meaning is provider-specific:
	// a mobile number, a bank account, a card token, or a wallet address.
	Account string
	// Scheme names the account's provider-specific scheme, such as a mobile network,
	// bank, or card brand, when the caller supplied one.
	Scheme string
	// Metadata carries provider-specific account details supplied with the request,
	// such as a bank branch code. It is passed through and never persisted.
	Metadata map[string]any
	// Name is the payment party's display name when supplied.
	Name string
	// Email is the payment party's email address when supplied.
	Email string
	// Description is the human-readable payment narrative.
	Description string
	// Amount is the payment amount in minor currency units.
	Amount int64
}

// ProviderPaymentResponse describes a payment request accepted by a provider.
type ProviderPaymentResponse struct {
	// ProviderReference identifies the operation in the provider's system.
	ProviderReference string `json:"provider_reference"`
	// Status is the normalized transaction status.
	Status string `json:"status"`
	// Message provides a human-readable description of the result.
	Message string `json:"message"`
	// Raw contains the original structured provider response when available.
	Raw map[string]any `json:"raw,omitempty"`
}

// ProviderTransactionStatus describes the current state of a provider transaction.
type ProviderTransactionStatus struct {
	// ProviderReference identifies the transaction in the provider's system.
	ProviderReference string `json:"provider_reference"`
	// Status is the normalized transaction status.
	Status string `json:"status"`
	// Message provides the provider status text or a fallback description.
	Message string `json:"message"`
}

// ProviderBalance contains a provider account's balances in minor currency units.
type ProviderBalance struct {
	// Currency is the ISO currency code for the balances.
	Currency string `json:"currency"`
	// Available is the amount currently available for transactions.
	Available int64 `json:"available"`
	// Ledger is the total ledger balance reported by the provider.
	Ledger int64 `json:"ledger"`
}

// ProviderWebhookEvent contains normalized data extracted from a provider webhook.
type ProviderWebhookEvent struct {
	// ProviderReference identifies the transaction in the provider's system.
	ProviderReference string `json:"provider_reference"`
	// Status is the normalized transaction status.
	Status string `json:"status"`
	// EventType identifies the normalized webhook event.
	EventType string `json:"event_type"`
	// ExternalReference identifies the transaction in the calling system.
	ExternalReference string `json:"external_reference,omitempty"`
	// Amount is the transaction amount in minor currency units when supplied.
	Amount *int64 `json:"amount,omitempty"`
	// Currency is the transaction's ISO currency code when supplied.
	Currency string `json:"currency,omitempty"`
	// Country is the country code associated with the transaction when supplied.
	Country string `json:"country,omitempty"`
	// Account identifies the account associated with the transaction when supplied.
	// Momobase compares it against the account recorded for the transaction, so a
	// provider that normalizes accounts must report the same form it normalized to.
	// Leave it empty to skip that check.
	Account string `json:"account,omitempty"`
	// Raw contains the decoded provider webhook payload.
	Raw map[string]any `json:"raw,omitempty"`
}

// PaymentProvider is the minimum contract every provider implements. Payment
// operations are exposed through the smaller optional interfaces below.
type PaymentProvider interface {
	// Capabilities returns the operations enabled by the provider's current configuration.
	Capabilities() []Capability
	// Init validates and applies provider configuration.
	Init(context.Context, ProviderConfig) error
}

// HealthChecker verifies that a provider can reach and authenticate with its upstream API.
type HealthChecker interface {
	// HealthCheck verifies that the provider can authenticate with its upstream API.
	HealthCheck(context.Context) error
}

// Collector is implemented by providers that collect customer payments.
type Collector interface {
	// Collect requests a payment from a customer.
	Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
}

// Disburser is implemented by providers that send recipient payments.
type Disburser interface {
	// Disburse requests a payment to a recipient.
	Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
}

// TransactionQuerier is implemented by providers that support status polling.
type TransactionQuerier interface {
	// QueryTransaction retrieves a transaction status by provider reference and country.
	QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error)
}

// BalanceQuerier is implemented by providers that expose account balances.
type BalanceQuerier interface {
	// QueryBalance retrieves an account balance for the given country.
	QueryBalance(context.Context, string) (*ProviderBalance, error)
}

// WebhookVerifier is implemented by providers that receive callbacks.
type WebhookVerifier interface {
	// VerifyWebhook normalizes a provider webhook payload and headers.
	VerifyWebhook(context.Context, []byte, map[string]string) (*ProviderWebhookEvent, error)
}

// RequestValidator is implemented by providers that validate and normalize a
// payment request before Momobase persists a transaction. Implementing it is
// optional: Momobase looks for it after selecting a route and skips the step when
// a provider does not implement it.
//
// This is where provider-specific knowledge of an account belongs. The engine
// treats Account as opaque, so a provider that requires a valid MSISDN, IBAN, or
// card number enforces it here and Momobase rejects the payment before any
// transaction row exists.
//
// ValidateRequest may rewrite Account and Scheme in place, for example to
// canonicalize an account identifier; the normalized value is what Momobase
// records and what webhook matching later compares against. It must not change
// any other field, and Momobase rejects a request whose amount, currency,
// country, reference, payment method, or transaction ID was modified.
type RequestValidator interface {
	// ValidateRequest validates and normalizes req. Returning an error rejects the
	// payment before it is persisted.
	ValidateRequest(context.Context, *PaymentRequest) error
}

// ErrCircuitOpen indicates that a provider request was rejected by an open circuit breaker.
var ErrCircuitOpen = errors.New("provider circuit breaker is open")

// ErrOperationUnsupported indicates that a provider does not implement an optional operation.
var ErrOperationUnsupported = errors.New("provider operation is not supported")
