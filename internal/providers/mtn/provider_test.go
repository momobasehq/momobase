package mtn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/providers"
)

func TestParseConfigDefaultsOverridesAndSecurity(t *testing.T) {
	cfg, err := parseConfig(providers.ProviderConfig{})
	if err != nil {
		t.Fatalf("parseConfig(defaults) error = %v", err)
	}
	if cfg.BaseURL != baseURL || cfg.TargetEnvironment != "sandbox" || cfg.Currency != "UGX" || cfg.Timeout != 30*time.Second {
		t.Fatalf("parseConfig(defaults) = %+v", cfg)
	}

	cfg, err = parseConfig(providers.ProviderConfig{
		"base_url":                    "http://localhost:8080/",
		"allow_insecure_http":         true,
		"target_environment":          "production",
		"currency":                    "rwf",
		"callback_url":                "https://callback.example.com",
		"timeout_seconds":             "11",
		"collection_subscription_key": "subscription",
		"collection_api_user":         "user",
		"collection_api_key":          "secret",
	})
	if err != nil {
		t.Fatalf("parseConfig(overrides) error = %v", err)
	}
	if cfg.BaseURL != "http://localhost:8080" || cfg.Currency != "RWF" || cfg.Timeout != 11*time.Second {
		t.Fatalf("parseConfig(overrides) = %+v", cfg)
	}
	if !cfg.Collection.valid() || cfg.Disbursement.valid() {
		t.Fatalf("parsed products = %+v, %+v", cfg.Collection, cfg.Disbursement)
	}

	if _, err := parseConfig(providers.ProviderConfig{"base_url": "http://api.example.com"}); err == nil {
		t.Fatal("parseConfig() accepted insecure HTTP without opt-in")
	}
}

func TestProviderCapabilitiesRequireCompleteProducts(t *testing.T) {
	p := New(nil).(*Provider)
	if err := p.Init(context.Background(), providers.ProviderConfig{
		"collection_subscription_key": "subscription",
		"collection_api_user":         "user",
		"collection_api_key":          "secret",
	}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0].ServiceType != domain.ServiceCollection {
		t.Fatalf("Capabilities() = %v", caps)
	}
	if _, err := p.Disburse(context.Background(), providers.PaymentRequest{}); err == nil {
		t.Fatal("Disburse() succeeded without disbursement credentials")
	}

	empty := New(nil).(*Provider)
	if err := empty.Init(context.Background(), providers.ProviderConfig{}); err != nil {
		t.Fatalf("Init(empty) error = %v", err)
	}
	if err := empty.HealthCheck(context.Background()); err == nil {
		t.Fatal("HealthCheck() succeeded without credentials")
	}
}

func TestProviderHTTPFlows(t *testing.T) {
	var tokenCalls atomic.Int32
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:secret"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/collection/token/":
			tokenCalls.Add(1)
			if r.Header.Get("Authorization") != basic || r.Header.Get("Ocp-Apim-Subscription-Key") != "subscription" {
				t.Errorf("token headers = %v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "expires_in": 3600})
		case "/collection/v1_0/requesttopay":
			if r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("X-Callback-Url") != "https://callback.example.com" {
				t.Errorf("payment headers = %v", r.Header)
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode payment request: %v", err)
				return
			}
			if note, _ := body["payerMessage"].(string); len(note) != 160 {
				t.Errorf("payerMessage length = %d", len(note))
			}
			w.WriteHeader(http.StatusAccepted)
		case "/collection/v1_0/requesttopay/provider-ref":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "SUCCESSFUL"})
		case "/collection/v1_0/account/balance":
			_ = json.NewEncoder(w).Encode(map[string]string{"currency": "UGX", "availableBalance": "5000"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	p := New(nil).(*Provider)
	err := p.Init(context.Background(), providers.ProviderConfig{
		"base_url":                    server.URL,
		"allow_insecure_http":         true,
		"callback_url":                "https://callback.example.com",
		"collection_subscription_key": "subscription",
		"collection_api_user":         "user",
		"collection_api_key":          "secret",
	})
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	payment, err := p.Collect(context.Background(), providers.PaymentRequest{
		Amount:      5000,
		Currency:    "UGX",
		Reference:   "order-1",
		Phone:       "256770000000",
		Description: strings.Repeat("x", 200),
	})
	if err != nil || payment.Status != domain.TxProcessing || payment.ProviderReference == "" {
		t.Fatalf("Collect() = %+v, %v", payment, err)
	}
	status, err := p.QueryTransaction(context.Background(), "provider-ref", "")
	if err != nil || status.Status != domain.TxSucceeded {
		t.Fatalf("QueryTransaction() = %+v, %v", status, err)
	}
	balance, err := p.QueryBalance(context.Background(), "")
	if err != nil || balance.Available != 5000 || balance.Currency != "UGX" {
		t.Fatalf("QueryBalance() = %+v, %v", balance, err)
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token endpoint called %d times", tokenCalls.Load())
	}
	if _, err := p.QueryTransaction(context.Background(), "", ""); err == nil {
		t.Fatal("QueryTransaction() accepted an empty reference")
	}
}

func TestVerifyWebhook(t *testing.T) {
	p := &Provider{}
	event, err := p.VerifyWebhook(context.Background(), []byte(`{
		"referenceId": "mtn-ref",
		"status": "SUCCESSFUL",
		"externalId": "order-1",
		"amount": "10.25",
		"currency": "USD",
		"payer": {"partyId": "256770000000"}
	}`), nil)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.ProviderReference != "mtn-ref" || event.Status != domain.TxSucceeded || event.Amount == nil || *event.Amount != 1025 {
		t.Fatalf("VerifyWebhook() = %+v", event)
	}
	if event.ExternalReference != "order-1" || event.Phone != "256770000000" {
		t.Fatalf("VerifyWebhook() identity fields = %+v", event)
	}
	if _, err := p.VerifyWebhook(context.Background(), []byte(`{"amount":"1.234","currency":"USD"}`), nil); err == nil {
		t.Fatal("VerifyWebhook() accepted an invalid amount")
	}
	if _, err := p.VerifyWebhook(context.Background(), []byte(`{`), nil); err == nil {
		t.Fatal("VerifyWebhook() accepted invalid JSON")
	}
}
