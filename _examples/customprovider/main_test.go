package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/providers"
)

const webhookSecret = "acme-webhook-signing-credential"

// acmeServer stands in for the Acme Pay API, answering the endpoints the
// provider calls and recording the last request body it received.
func acmeServer(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()
	received := map[string]any{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/ping", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	for _, path := range []string{"POST /v1/collections", "POST /v1/payouts"} {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&received)
			_, _ = w.Write([]byte(`{"id":"acme_1","status":"SUCCESSFUL","message":"done"}`))
		})
	}
	mux.HandleFunc("GET /v1/transactions/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"PENDING","message":"awaiting customer"}`))
	})
	mux.HandleFunc("GET /v1/balance", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("country") != "UG" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"currency":"UGX","available":"1500","ledger":"1800"}`))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, &received
}

func newTestProvider(t *testing.T, baseURL string) *acmeProvider {
	t.Helper()
	provider, ok := newAcmeProvider(slog.New(slog.NewTextHandler(io.Discard, nil))).(*acmeProvider)
	if !ok {
		t.Fatal("newAcmeProvider() did not return *acmeProvider")
	}
	config := providers.ProviderConfig{
		"api_key":        "test-api-key",
		"webhook_secret": webhookSecret,
		"base_url":       baseURL,
	}
	if err := provider.Init(context.Background(), config); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return provider
}

func TestInitRequiresAnAPIKeyAndDefaultsTheBaseURL(t *testing.T) {
	provider := newAcmeProvider(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := provider.Init(context.Background(), providers.ProviderConfig{}); err == nil {
		t.Fatal("Init() without an api key = nil, want an error")
	}

	acme, ok := provider.(*acmeProvider)
	if !ok {
		t.Fatal("newAcmeProvider() did not return *acmeProvider")
	}
	if err := provider.Init(context.Background(), providers.ProviderConfig{"api_key": "k"}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if !strings.HasPrefix(acme.baseURL, "https://") {
		t.Errorf("baseURL = %q, want the documented default", acme.baseURL)
	}
}

func TestCapabilitiesCoverBothServices(t *testing.T) {
	server, _ := acmeServer(t)
	caps := newTestProvider(t, server.URL).Capabilities()
	for _, service := range []string{providers.ServiceCollection, providers.ServiceDisbursement} {
		if !slices.ContainsFunc(caps, func(c providers.Capability) bool { return c.ServiceType == service }) {
			t.Errorf("Capabilities() is missing %s", service)
		}
	}
}

func TestHealthCheckAuthenticates(t *testing.T) {
	server, _ := acmeServer(t)
	if err := newTestProvider(t, server.URL).HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}

	unauthorized, ok := newAcmeProvider(slog.New(slog.NewTextHandler(io.Discard, nil))).(*acmeProvider)
	if !ok {
		t.Fatal("newAcmeProvider() did not return *acmeProvider")
	}
	config := providers.ProviderConfig{"api_key": "wrong", "base_url": server.URL}
	if err := unauthorized.Init(context.Background(), config); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := unauthorized.HealthCheck(context.Background()); err == nil {
		t.Fatal("HealthCheck() with a bad key = nil, want an error")
	}
}

func TestCollectAndDisburseFormatTheRequest(t *testing.T) {
	server, received := acmeServer(t)
	provider := newTestProvider(t, server.URL)
	request := providers.PaymentRequest{
		TransactionID: "txn_1",
		Amount:        150000,
		Currency:      "UGX",
		Country:       "UG",
		Reference:     "ORDER-1",
		Account:       "256770000000",
		Description:   "test payment",
	}

	for name, call := range map[string]func() (*providers.ProviderPaymentResponse, error){
		"collect": func() (*providers.ProviderPaymentResponse, error) {
			return provider.Collect(context.Background(), request)
		},
		"disburse": func() (*providers.ProviderPaymentResponse, error) {
			return provider.Disburse(context.Background(), request)
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := call()
			if err != nil {
				t.Fatalf("%s() error = %v", name, err)
			}
			if result.ProviderReference != "acme_1" || result.Status != providers.TxSucceeded {
				t.Fatalf("%s() = %+v, want a succeeded acme_1 response", name, result)
			}
			// UGX is a zero-decimal currency, so minor units reach the API unscaled.
			if got := (*received)["amount"]; got != "150000" {
				t.Errorf("amount sent = %v, want the zero-decimal formatting", got)
			}
			if got := (*received)["msisdn"]; got != "256770000000" {
				t.Errorf("msisdn sent = %v, want the request account", got)
			}
		})
	}
}

func TestQueryTransactionAndBalance(t *testing.T) {
	server, _ := acmeServer(t)
	provider := newTestProvider(t, server.URL)

	status, err := provider.QueryTransaction(context.Background(), "acme_1", "UG")
	if err != nil {
		t.Fatalf("QueryTransaction() error = %v", err)
	}
	if status.Status != providers.TxProcessing || status.ProviderReference != "acme_1" {
		t.Fatalf("QueryTransaction() = %+v, want processing for acme_1", status)
	}

	balance, err := provider.QueryBalance(context.Background(), "UG")
	if err != nil {
		t.Fatalf("QueryBalance() error = %v", err)
	}
	if balance.Currency != "UGX" || balance.Available != 1500 || balance.Ledger != 1800 {
		t.Fatalf("QueryBalance() = %+v, want 1500/1800 UGX", balance)
	}

	if _, err = provider.QueryBalance(context.Background(), "KE"); err == nil {
		t.Error("QueryBalance(KE) error = nil, want the server rejection")
	}
}

func signed(payload []byte) map[string]string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(payload)
	return map[string]string{"X-Acme-Signature": hex.EncodeToString(mac.Sum(nil))}
}

func TestVerifyWebhook(t *testing.T) {
	server, _ := acmeServer(t)
	provider := newTestProvider(t, server.URL)
	payload := []byte(`{"id":"acme_1","event":"payment.completed","status":"SUCCESSFUL",` +
		`"reference":"ORDER-1","amount":"2500","currency":"UGX","country":"UG","msisdn":"256770000000"}`)

	event, err := provider.VerifyWebhook(context.Background(), payload, signed(payload))
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.ProviderReference != "acme_1" || event.Status != providers.TxSucceeded {
		t.Fatalf("VerifyWebhook() = %+v, want a succeeded acme_1 event", event)
	}
	if event.Amount == nil || *event.Amount != 2500 {
		t.Fatalf("VerifyWebhook() amount = %v, want 2500", event.Amount)
	}
	if event.ExternalReference != "ORDER-1" || event.Account != "256770000000" {
		t.Errorf("VerifyWebhook() = %+v, want the reference and msisdn carried through", event)
	}

	if _, err = provider.VerifyWebhook(context.Background(), payload, map[string]string{"X-Acme-Signature": "bad"}); err == nil {
		t.Error("VerifyWebhook() with a bad signature = nil, want an error")
	}
	broken := []byte(`{"amount":"1.234","currency":"USD"}`)
	if _, err = provider.VerifyWebhook(context.Background(), broken, signed(broken)); err == nil {
		t.Error("VerifyWebhook() with an unparseable amount = nil, want an error")
	}
	invalid := []byte(`{nope`)
	if _, err = provider.VerifyWebhook(context.Background(), invalid, signed(invalid)); err == nil {
		t.Error("VerifyWebhook() with invalid JSON = nil, want an error")
	}

	unsigned, ok := newAcmeProvider(slog.New(slog.NewTextHandler(io.Discard, nil))).(*acmeProvider)
	if !ok {
		t.Fatal("newAcmeProvider() did not return *acmeProvider")
	}
	if err = unsigned.Init(context.Background(), providers.ProviderConfig{"api_key": "k"}); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err = unsigned.VerifyWebhook(context.Background(), payload, signed(payload)); err == nil {
		t.Error("VerifyWebhook() without a configured credential = nil, want an error")
	}
}

func TestValidateRequestNormalizesTheAccount(t *testing.T) {
	server, _ := acmeServer(t)
	provider := newTestProvider(t, server.URL)
	var validator providers.RequestValidator = provider
	for _, account := range []string{"0770000000", "+256770000000", "256770000000", " 077 000 0000 ", "770000000"} {
		req := providers.PaymentRequest{Country: "UG", Account: account}
		if err := validator.ValidateRequest(context.Background(), &req); err != nil {
			t.Fatalf("ValidateRequest(%q) error = %v", account, err)
		}
		if req.Account != "256770000000" {
			t.Errorf("ValidateRequest(%q) account = %q, want the canonical E.164 digits", account, req.Account)
		}
	}

	for name, req := range map[string]providers.PaymentRequest{
		"letters":         {Country: "UG", Account: "not-a-number"},
		"too short":       {Country: "UG", Account: "0770000"},
		"too long":        {Country: "UG", Account: "07700000001234"},
		"blank account":   {Country: "UG", Account: "   "},
		"uncovered":       {Country: "GB", Account: "0770000000"},
		"country missing": {Account: "0770000000"},
	} {
		if err := validator.ValidateRequest(context.Background(), &req); err == nil {
			t.Errorf("ValidateRequest(%s) = nil, want an unusable account rejected", name)
		}
	}
}
