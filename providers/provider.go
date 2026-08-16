package providers

import (
	"context"
	"errors"
)

// Capability identifies a payment service supported by a provider.
//
// A capability names the service only — collection or disbursement. Which rails an
// account serves is a routing decision, expressed by the payment routes an operator
// creates for it, so a provider declares what it can do rather than the vocabulary
// it does it under.
type Capability struct {
	// ServiceType identifies the payment service, such as collection or disbursement.
	ServiceType string `json:"service_type"`
}

// ProviderConfig contains the provider-specific values used during initialization.
type ProviderConfig map[string]any

// PaymentRequest contains the normalized details of a provider payment operation.
type PaymentRequest struct {
	// TransactionID identifies the Momobase transaction associated with the request.
	TransactionID string
	// PaymentMethod identifies the payment rail the route selected for this request.
	// It is the free-form method recorded on the route, and a provider that serves
	// more than one rail dispatches on it.
	PaymentMethod string
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

// PaymentProvider defines the operations implemented by a payment provider.
type PaymentProvider interface {
	// Capabilities returns the operations enabled by the provider's current configuration.
	Capabilities() []Capability
	// Init validates and applies provider configuration.
	Init(context.Context, ProviderConfig) error
	// HealthCheck verifies that the provider can authenticate with its upstream API.
	HealthCheck(context.Context) error
	// Collect requests a payment from a customer.
	Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	// Disburse requests a payment to a recipient.
	Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	// QueryTransaction retrieves a transaction status by provider reference and country.
	QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error)
	// QueryBalance retrieves an account balance for the given country.
	QueryBalance(context.Context, string) (*ProviderBalance, error)
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
