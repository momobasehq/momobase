package airtel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

func TestParseConfigDefaultsOverridesAndSecurity(t *testing.T) {
	cfg, err := parseConfig(providers.ProviderConfig{})
	if err != nil {
		t.Fatalf("parseConfig(defaults) error = %v", err)
	}
	if cfg.BaseURL != "https://openapiuat.airtel.africa" || cfg.Currency != "UGX" || cfg.Timeout != 30*time.Second {
		t.Fatalf("parseConfig(defaults) = %+v", cfg)
	}
	if !cfg.CollectionEnabled || !cfg.DisbursementEnabled {
		t.Fatalf("default capabilities disabled: %+v", cfg)
	}

	cfg, err = parseConfig(providers.ProviderConfig{
		"base_url":            "http://localhost:8080/",
		"allow_insecure_http": true,
		"client_id":           "client",
		"client_secret":       "secret",
		"currency":            "rwf",
		"timeout_seconds":     12,
		"collection_path":     "collect",
		"collection_enabled":  false,
	})
	if err != nil {
		t.Fatalf("parseConfig(overrides) error = %v", err)
	}
	if cfg.BaseURL != "http://localhost:8080" || cfg.Currency != "RWF" || cfg.Timeout != 12*time.Second {
		t.Fatalf("parseConfig(overrides) = %+v", cfg)
	}
	if cfg.CollectionPath != "/collect" || cfg.CollectionEnabled {
		t.Fatalf("collection overrides = %+v", cfg)
	}

	if _, err := parseConfig(providers.ProviderConfig{"base_url": "http://api.example.com"}); err == nil {
		t.Fatal("parseConfig() accepted insecure HTTP without opt-in")
	}
}

func TestProviderCapabilitiesRequireCredentialsAndFlags(t *testing.T) {
	p := New(nil).(*Provider)
	if caps := p.Capabilities(); len(caps) != 0 {
		t.Fatalf("Capabilities() without credentials = %v", caps)
	}
	if err := p.Init(context.Background(), providers.ProviderConfig{
		"client_id":            "client",
		"client_secret":        "secret",
		"collection_enabled":   true,
		"disbursement_enabled": false,
	}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0].ServiceType != domain.ServiceCollection {
		t.Fatalf("Capabilities() = %v", caps)
	}
	if _, err := p.Disburse(context.Background(), providers.PaymentRequest{}); err == nil {
		t.Fatal("Disburse() succeeded while disabled")
	}
}

func TestProviderHTTPFlows(t *testing.T) {
	var tokenCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/oauth2/token":
			tokenCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "expires_in": 3600})
		case "/collect":
			if r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("X-Country") != "UG" {
				t.Errorf("collection headers = %v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"transaction": map[string]any{"id": "airtel-ref"}})
		case "/status/airtel-ref":
			_ = json.NewEncoder(w).Encode(map[string]any{"transaction": map[string]any{"status": "SUCCESS"}})
		case "/balance":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"currency": "UGX", "balance": "2500"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	p := New(nil).(*Provider)
	err := p.Init(context.Background(), providers.ProviderConfig{
		"base_url":             server.URL,
		"allow_insecure_http":  true,
		"client_id":            "client",
		"client_secret":        "secret",
		"collection_path":      "/collect",
		"status_path_template": "/status/{id}",
		"balance_path":         "/balance",
		"disbursement_enabled": false,
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	payment, err := p.Collect(context.Background(), providers.PaymentRequest{
		Amount:      2500,
		Currency:    "UGX",
		Country:     "ug",
		Reference:   "order-1",
		Phone:       "256770000000",
		Description: "test",
	})
	if err != nil || payment.ProviderReference != "airtel-ref" || payment.Status != domain.TxProcessing {
		t.Fatalf("Collect() = %+v, %v", payment, err)
	}
	status, err := p.QueryTransaction(context.Background(), "airtel-ref", "UG")
	if err != nil || status.Status != domain.TxSucceeded {
		t.Fatalf("QueryTransaction() = %+v, %v", status, err)
	}
	balance, err := p.QueryBalance(context.Background(), "UG")
	if err != nil || balance.Available != 2500 || balance.Currency != "UGX" {
		t.Fatalf("QueryBalance() = %+v, %v", balance, err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint called %d times", tokenCalls.Load())
	}
	if _, err := p.QueryTransaction(context.Background(), "", "UG"); err == nil {
		t.Fatal("QueryTransaction() accepted an empty reference")
	}
}

func TestVerifyWebhook(t *testing.T) {
	p := &Provider{}
	event, err := p.VerifyWebhook(context.Background(), []byte(`{
		"transaction": {
			"id": "airtel-ref",
			"status": "TS",
			"amount": "12.34",
			"currency": "USD",
			"reference": "order-1",
			"msisdn": "256770000000"
		}
	}`), nil)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.ProviderReference != "airtel-ref" || event.Status != domain.TxSucceeded || event.Amount == nil || *event.Amount != 1234 {
		t.Fatalf("VerifyWebhook() = %+v", event)
	}
	if event.ExternalReference != "order-1" || event.Phone != "256770000000" {
		t.Fatalf("VerifyWebhook() identity fields = %+v", event)
	}

	if _, err := p.VerifyWebhook(context.Background(), []byte(`{"amount":"1.234","currency":"USD"}`), nil); err == nil {
		t.Fatal("VerifyWebhook() accepted an invalid amount")
	}
	if _, err := p.VerifyWebhook(context.Background(), []byte(`{`), nil); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("VerifyWebhook(invalid JSON) error = %v", err)
	}
}
