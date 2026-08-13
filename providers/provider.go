package providers

import (
	"context"
	"errors"
)

// Capability identifies a service and payment-method combination supported by a provider.
type Capability struct {
	// ServiceType identifies the payment service, such as collection or disbursement.
	ServiceType string `json:"service_type"`
	// PaymentMethod identifies the payment rail used for the service.
	PaymentMethod string `json:"payment_method"`
}

// ProviderConfig contains the provider-specific values used during initialization.
type ProviderConfig map[string]any

// PaymentRequest contains the normalized details of a provider payment operation.
type PaymentRequest struct {
	// TransactionID identifies the Momobase transaction associated with the request.
	TransactionID string
	// Currency is the ISO currency code for the payment amount.
	Currency string
	// Country is the country code for the payment party.
	Country string
	// Reference is the external reference supplied to the provider.
	Reference string
	// Phone is the mobile number of the payer or payee.
	Phone string
	// Network identifies the mobile network associated with Phone.
	Network string
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
	// Phone is the mobile number associated with the transaction when supplied.
	Phone string `json:"phone,omitempty"`
	// Raw contains the decoded provider webhook payload.
	Raw map[string]any `json:"raw,omitempty"`
}

// PaymentProvider defines the operations implemented by a mobile-money provider.
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

// ErrCircuitOpen indicates that a provider request was rejected by an open circuit breaker.
var ErrCircuitOpen = errors.New("provider circuit breaker is open")
