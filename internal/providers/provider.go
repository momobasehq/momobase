package providers

import (
	"context"
	"errors"
)

type Capability struct {
	ServiceType   string `json:"service_type"`
	PaymentMethod string `json:"payment_method"`
}

type ProviderConfig map[string]any

type PaymentRequest struct {
	TransactionID string
	Currency      string
	Country       string
	Reference     string
	Phone         string
	Network       string
	Description   string
	Amount        int64
}

type ProviderPaymentResponse struct {
	ProviderReference string         `json:"provider_reference"`
	Status            string         `json:"status"`
	Message           string         `json:"message"`
	Raw               map[string]any `json:"raw,omitempty"`
}

type ProviderTransactionStatus struct {
	ProviderReference string `json:"provider_reference"`
	Status            string `json:"status"`
	Message           string `json:"message"`
}

type ProviderBalance struct {
	Currency  string `json:"currency"`
	Available int64  `json:"available"`
	Ledger    int64  `json:"ledger"`
}

type ProviderWebhookEvent struct {
	ProviderReference string         `json:"provider_reference"`
	Status            string         `json:"status"`
	EventType         string         `json:"event_type"`
	ExternalReference string         `json:"external_reference,omitempty"`
	Amount            *int64         `json:"amount,omitempty"`
	Currency          string         `json:"currency,omitempty"`
	Country           string         `json:"country,omitempty"`
	Phone             string         `json:"phone,omitempty"`
	Raw               map[string]any `json:"raw,omitempty"`
}

type PaymentProvider interface {
	Capabilities() []Capability
	Init(context.Context, ProviderConfig) error
	HealthCheck(context.Context) error
	Collect(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	Disburse(context.Context, PaymentRequest) (*ProviderPaymentResponse, error)
	QueryTransaction(context.Context, string, string) (*ProviderTransactionStatus, error)
	QueryBalance(context.Context, string) (*ProviderBalance, error)
	VerifyWebhook(context.Context, []byte, map[string]string) (*ProviderWebhookEvent, error)
}

var ErrCircuitOpen = errors.New("provider circuit breaker is open")
