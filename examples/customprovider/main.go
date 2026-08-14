// Package main runs a Momobase server extended with a custom payment provider.
//
// The provider below targets a fictional "Acme Pay" API and shows the whole
// contract: configuration, capabilities, payments, status and balance queries,
// and webhook verification. Register it under a provider code, then create and
// activate an account for that code through the Admin API.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/momobasehq/momobase"
)

// acmeProvider implements momobase.PaymentProvider for the Acme Pay API.
type acmeProvider struct {
	log           *slog.Logger
	client        *http.Client
	baseURL       string
	apiKey        string
	webhookSecret string
}

// newAcmeProvider is the factory Momobase calls to build a provider instance.
// The logger is never nil and is already tagged with the provider code.
func newAcmeProvider(log *slog.Logger) momobase.PaymentProvider {
	return &acmeProvider{log: log, client: &http.Client{Timeout: 30 * time.Second}}
}

// Init applies the configuration recorded for an Acme Pay account. It is called
// whenever an account is created, updated, or reloaded, so it must not retain
// state from a previous call.
func (p *acmeProvider) Init(_ context.Context, cfg momobase.ProviderConfig) error {
	p.apiKey = momobase.ConfigString(cfg, "api_key")
	if p.apiKey == "" {
		return errors.New("acme_pay: api_key is required")
	}
	p.baseURL = momobase.First(momobase.ConfigString(cfg, "base_url"), "https://api.acme.example")
	p.webhookSecret = momobase.ConfigString(cfg, "webhook_secret")
	return nil
}

// Capabilities reports the operations this configuration can perform. Momobase
// only routes payments to a provider that reports a matching capability.
func (p *acmeProvider) Capabilities() []momobase.Capability {
	return []momobase.Capability{
		{ServiceType: momobase.ServiceCollection, PaymentMethod: momobase.PaymentMethodMomo},
		{ServiceType: momobase.ServiceDisbursement, PaymentMethod: momobase.PaymentMethodMomo},
	}
}

// HealthCheck verifies that the configured credentials still authenticate.
func (p *acmeProvider) HealthCheck(ctx context.Context) error {
	return momobase.DoJSON(ctx, p.client, http.MethodGet, p.baseURL+"/v1/ping", p.headers(), nil, nil)
}

// Collect requests a payment from a customer.
func (p *acmeProvider) Collect(
	ctx context.Context,
	req momobase.PaymentRequest,
) (*momobase.ProviderPaymentResponse, error) {
	return p.pay(ctx, "/v1/collections", req)
}

// Disburse sends a payment to a recipient.
func (p *acmeProvider) Disburse(
	ctx context.Context,
	req momobase.PaymentRequest,
) (*momobase.ProviderPaymentResponse, error) {
	return p.pay(ctx, "/v1/payouts", req)
}

func (p *acmeProvider) pay(
	ctx context.Context,
	path string,
	req momobase.PaymentRequest,
) (*momobase.ProviderPaymentResponse, error) {
	// Amounts reach the provider in minor units and are formatted for the API.
	body := map[string]any{
		"reference": momobase.First(req.Reference, momobase.RandomRef("acme")),
		"amount":    momobase.FormatAmountMinor(req.Amount, req.Currency),
		"currency":  req.Currency,
		"country":   req.Country,
		"msisdn":    req.Phone,
		"narration": req.Description,
	}
	var out struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := momobase.DoJSON(ctx, p.client, http.MethodPost, p.baseURL+path, p.headers(), body, &out); err != nil {
		return nil, err
	}
	return &momobase.ProviderPaymentResponse{
		ProviderReference: out.ID,
		// PaymentStatus maps common provider vocabularies onto Momobase
		// statuses; map bespoke values yourself and return a Tx constant.
		Status:  momobase.PaymentStatus(out.Status),
		Message: out.Message,
	}, nil
}

// QueryTransaction retrieves the current status of a submitted transaction. It
// backs both the status API and the reconciliation worker.
func (p *acmeProvider) QueryTransaction(
	ctx context.Context,
	providerReference string,
	_ string,
) (*momobase.ProviderTransactionStatus, error) {
	var out struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	url := p.baseURL + "/v1/transactions/" + providerReference
	if err := momobase.DoJSON(ctx, p.client, http.MethodGet, url, p.headers(), nil, &out); err != nil {
		return nil, err
	}
	return &momobase.ProviderTransactionStatus{
		ProviderReference: providerReference,
		Status:            momobase.PaymentStatus(out.Status),
		Message:           out.Message,
	}, nil
}

// QueryBalance retrieves the account balance for a country in minor units.
func (p *acmeProvider) QueryBalance(ctx context.Context, country string) (*momobase.ProviderBalance, error) {
	var out struct {
		Currency  string `json:"currency"`
		Available string `json:"available"`
		Ledger    string `json:"ledger"`
	}
	url := fmt.Sprintf("%s/v1/balance?country=%s", p.baseURL, country)
	if err := momobase.DoJSON(ctx, p.client, http.MethodGet, url, p.headers(), nil, &out); err != nil {
		return nil, err
	}
	available, err := momobase.ParseAmountToMinor(out.Available, out.Currency)
	if err != nil {
		return nil, err
	}
	ledger, err := momobase.ParseAmountToMinor(out.Ledger, out.Currency)
	if err != nil {
		return nil, err
	}
	return &momobase.ProviderBalance{Currency: out.Currency, Available: available, Ledger: ledger}, nil
}

// VerifyWebhook authenticates an incoming callback and normalizes it. Returning
// an error rejects the delivery, so verify the signature before trusting the
// payload.
func (p *acmeProvider) VerifyWebhook(
	_ context.Context,
	payload []byte,
	headers map[string]string,
) (*momobase.ProviderWebhookEvent, error) {
	if p.webhookSecret == "" {
		return nil, errors.New("acme_pay: webhook_secret is required to verify callbacks")
	}
	mac := hmac.New(sha256.New, []byte(p.webhookSecret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(headers["X-Acme-Signature"])) {
		return nil, errors.New("acme_pay: webhook signature mismatch")
	}
	var body struct {
		ID        string `json:"id"`
		Event     string `json:"event"`
		Status    string `json:"status"`
		Reference string `json:"reference"`
		Amount    string `json:"amount"`
		Currency  string `json:"currency"`
		Country   string `json:"country"`
		MSISDN    string `json:"msisdn"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	amount, err := momobase.OptionalAmount(body.Amount, body.Currency)
	if err != nil {
		return nil, err
	}
	raw := map[string]any{}
	_ = json.Unmarshal(payload, &raw)
	return &momobase.ProviderWebhookEvent{
		ProviderReference: body.ID,
		Status:            momobase.PaymentStatus(body.Status),
		EventType:         body.Event,
		ExternalReference: body.Reference,
		Amount:            amount,
		Currency:          body.Currency,
		Country:           body.Country,
		Phone:             body.MSISDN,
		Raw:               raw,
	}, nil
}

func (p *acmeProvider) headers() map[string]string {
	return map[string]string{"Authorization": "Bearer " + p.apiKey}
}

func main() {
	// Configuration is read from the environment; options override it.
	instance, err := momobase.New(
		momobase.WithProvider("acme_pay", newAcmeProvider),
		momobase.WithAddr(":9090"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = instance.Close() }()

	// Run serves until the process is interrupted. Use Serve(ctx) to control
	// shutdown yourself, or Handler() to mount Momobase in an existing server.
	if err := instance.Run(); err != nil {
		log.Fatal(err)
	}
}
