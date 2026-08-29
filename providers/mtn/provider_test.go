package mtn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

const testWebhookSecret = "long-random-webhook-secret"

func providerConfig(values map[string]string) providers.ProviderConfig {
	config := providers.ProviderConfig{}
	for key, value := range values {
		config[key] = value
	}
	return config
}

func testProvider(t *testing.T, overrides map[string]string) *Provider {
	t.Helper()
	config := providerConfig(map[string]string{
		"environment":                 "production",
		"base_url":                    "https://mtn.test",
		"target_environment":          "mtnuganda",
		"api_user":                    "api-user",
		"api_key":                     "api-key",
		"collection_subscription_key": "collection-key",
		"webhook_secret":              testWebhookSecret,
	})
	config = mergeConfig(config, overrides)
	provider, ok := New(slog.New(slog.NewTextHandler(io.Discard, nil))).(*Provider)
	if !ok {
		t.Fatal("New() did not return *Provider")
	}
	if err := provider.Init(context.Background(), config); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return provider
}

func payment(id string) providers.PaymentRequest {
	return providers.PaymentRequest{
		TransactionID: id,
		Currency:      "EUR",
		Country:       "UG",
		Reference:     "invoice-" + id,
		Account:       "256770000000",
		Description:   "payment " + id,
		Amount:        1250,
	}
}

func TestParseConfigDefaultsAndCapabilities(t *testing.T) {
	cfg, err := parseConfig(providerConfig(map[string]string{
		"environment":                 "sandbox",
		"collection_subscription_key": "collection-key",
		"webhook_secret":              testWebhookSecret,
	}))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Environment != "sandbox" {
		t.Errorf("Environment = %q, want sandbox", cfg.Environment)
	}
	if cfg.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, defaultBaseURL)
	}
	if cfg.TargetEnvironment != "sandbox" {
		t.Errorf("TargetEnvironment = %q, want sandbox", cfg.TargetEnvironment)
	}
	if cfg.APIUser != "" || cfg.APIKey != "" {
		t.Errorf("sandbox credentials = %q/%q, want empty for provisioning", cfg.APIUser, cfg.APIKey)
	}
	if cfg.BalanceService != domain.ServiceCollection {
		t.Errorf("BalanceService = %q, want collection", cfg.BalanceService)
	}

	provider := testProvider(t, nil)
	capabilities := provider.Capabilities()
	if len(capabilities) != 1 || capabilities[0].ServiceType != domain.ServiceCollection {
		t.Fatalf("Capabilities() = %v, want collection", capabilities)
	}

	provider = testProvider(t, map[string]string{
		"collection_subscription_key":   "",
		"disbursement_subscription_key": "disbursement-key",
	})
	if got := provider.config().BalanceService; got != domain.ServiceDisbursement {
		t.Errorf("BalanceService = %q, want disbursement", got)
	}
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	valid := providerConfig(map[string]string{
		"environment":                 "production",
		"base_url":                    "https://mtn.example.com",
		"target_environment":          "mtnuganda",
		"api_user":                    "api-user",
		"api_key":                     "api-key",
		"collection_subscription_key": "collection-key",
		"webhook_secret":              testWebhookSecret,
	})
	tests := map[string]providers.ProviderConfig{
		"production missing credentials": mergeConfig(valid, map[string]string{
			"api_user": "",
			"api_key":  "",
		}),
		"production missing base URL": mergeConfig(valid, map[string]string{
			"base_url": "",
		}),
		"production missing target environment": mergeConfig(valid, map[string]string{
			"target_environment": "",
		}),
		"invalid environment": mergeConfig(valid, map[string]string{
			"environment": "staging",
		}),
		"sandbox partial credentials": mergeConfig(valid, map[string]string{
			"environment": "sandbox",
			"api_key":     "",
		}),
		"missing webhook secret": mergeConfig(valid, map[string]string{
			"webhook_secret": "",
		}),
		"missing subscription": mergeConfig(valid, map[string]string{
			"collection_subscription_key": "",
		}),
		"insecure remote base URL": mergeConfig(valid, map[string]string{
			"base_url": "http://example.com",
		}),
		"invalid callback URL": mergeConfig(valid, map[string]string{
			"callback_url": "/webhooks/mtn",
		}),
		"invalid provider callback host": mergeConfig(valid, map[string]string{
			"provider_callback_host": "127.0.0.1",
		}),
		"disabled balance service": mergeConfig(valid, map[string]string{
			"balance_service": "disbursement",
		}),
		"header injection": mergeConfig(valid, map[string]string{
			"target_environment": "sandbox\r\nX-Injected: yes",
		}),
	}
	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(config); err == nil {
				t.Fatalf("parseConfig() error = nil, want an error")
			}
		})
	}
}

func TestInitAutoProvisionsSandboxCredentials(t *testing.T) {
	var apiUser string
	userCalls := 0
	keyCalls := 0
	tokenCalls := 0
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1_0/apiuser", func(w http.ResponseWriter, r *http.Request) {
		userCalls++
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "collection-key" {
			t.Errorf("subscription key = %q, want collection-key", got)
		}
		apiUser = r.Header.Get("X-Reference-Id")
		id, err := uuid.Parse(apiUser)
		if err != nil || id.Version() != 4 {
			t.Errorf("X-Reference-Id = %q, want a version 4 UUID", apiUser)
		}
		var body sandboxUserRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if body.ProviderCallbackHost != "hooks.example.com" {
			t.Errorf("provider callback host = %q, want hooks.example.com", body.ProviderCallbackHost)
		}
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("POST /v1_0/apiuser/{apiUser}/apikey", func(w http.ResponseWriter, r *http.Request) {
		keyCalls++
		if got := r.PathValue("apiUser"); got != apiUser {
			t.Errorf("API user path = %q, want %q", got, apiUser)
		}
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "collection-key" {
			t.Errorf("subscription key = %q, want collection-key", got)
		}
		if got := r.Header.Get("X-Reference-Id"); got != "" {
			t.Errorf("X-Reference-Id = %q, want omitted", got)
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(t, w, sandboxAPIKeyResponse{APIKey: "sandbox-api-key"})
	})
	mux.HandleFunc("POST /collection/token/", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		assertTokenRequestCredentials(t, r, "collection-key", apiUser+":sandbox-api-key")
		writeJSON(t, w, map[string]any{"access_token": "sandbox-token", "expires_in": 3600})
	})

	provider, ok := New(slog.New(slog.NewTextHandler(io.Discard, nil))).(*Provider)
	if !ok {
		t.Fatal("New() did not return *Provider")
	}
	provider.client = &http.Client{Transport: handlerTransport{handler: mux}}
	err := provider.Init(context.Background(), providerConfig(map[string]string{
		"environment":                 "sandbox",
		"base_url":                    "https://mtn.test",
		"callback_url":                "https://hooks.example.com/mtn",
		"collection_subscription_key": "collection-key",
		"webhook_secret":              testWebhookSecret,
	}))
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if got := provider.config(); got.APIUser != apiUser || got.APIKey != "sandbox-api-key" {
		t.Errorf("provisioned credentials = %q/%q, want generated user and key", got.APIUser, got.APIKey)
	}
	if err := provider.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if userCalls != 1 || keyCalls != 1 || tokenCalls != 1 {
		t.Errorf("calls = user %d, key %d, token %d; want one each", userCalls, keyCalls, tokenCalls)
	}
}

func TestInitReusesProvidedSandboxCredentials(t *testing.T) {
	provider := testProvider(t, map[string]string{
		"environment":        "sandbox",
		"target_environment": "sandbox",
	})
	if got := provider.config(); got.APIUser != "api-user" || got.APIKey != "api-key" {
		t.Errorf("credentials = %q/%q, want provided sandbox credentials", got.APIUser, got.APIKey)
	}
}

func TestValidateRequestNormalizesMSISDNAndScheme(t *testing.T) {
	provider := testProvider(t, nil)
	req := payment("normalize")
	req.Account = "+256 (770) 000-000"
	req.Scheme = "MTN"
	if err := provider.ValidateRequest(context.Background(), &req); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
	if req.Account != "256770000000" || req.Scheme != "mtn_momo" {
		t.Fatalf("ValidateRequest() normalized account/scheme = %q/%q", req.Account, req.Scheme)
	}

	for name, mutate := range map[string]func(*providers.PaymentRequest){
		"non-numeric account": func(req *providers.PaymentRequest) { req.Account = "25677invalid" },
		"short account":       func(req *providers.PaymentRequest) { req.Account = "1234" },
		"local account":       func(req *providers.PaymentRequest) { req.Account = "0770000000" },
		"wrong scheme":        func(req *providers.PaymentRequest) { req.Scheme = "airtel" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := payment(name)
			mutate(&invalid)
			if err := provider.ValidateRequest(context.Background(), &invalid); err == nil {
				t.Fatal("ValidateRequest() error = nil, want an error")
			}
		})
	}
}

func TestProviderAPIFlowsAndTokenCaching(t *testing.T) {
	requests := map[string]paymentRequest{}
	tokenCalls := map[string]int{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /collection/token/", func(w http.ResponseWriter, r *http.Request) {
		assertTokenRequest(t, r, "collection-key")
		tokenCalls[domain.ServiceCollection]++
		writeJSON(t, w, map[string]any{"access_token": "collection-token", "expires_in": 3600})
	})
	mux.HandleFunc("POST /disbursement/token/", func(w http.ResponseWriter, r *http.Request) {
		assertTokenRequest(t, r, "disbursement-key")
		tokenCalls[domain.ServiceDisbursement]++
		writeJSON(t, w, map[string]any{"access_token": "disbursement-token", "expires_in": 3600})
	})
	mux.HandleFunc("POST /collection/v1_0/requesttopay", func(w http.ResponseWriter, r *http.Request) {
		requests[domain.ServiceCollection] = decodePayment(t, r, "collection-token", "collection-key")
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("POST /disbursement/v1_0/transfer", func(w http.ResponseWriter, r *http.Request) {
		requests[domain.ServiceDisbursement] = decodePayment(t, r, "disbursement-token", "disbursement-key")
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /collection/v1_0/requesttopay/{id}", func(w http.ResponseWriter, r *http.Request) {
		assertAPIHeaders(t, r, "collection-token", "collection-key")
		writeJSON(t, w, map[string]any{"status": "SUCCESSFUL"})
	})
	mux.HandleFunc("GET /disbursement/v1_0/transfer/{id}", func(w http.ResponseWriter, r *http.Request) {
		assertAPIHeaders(t, r, "disbursement-token", "disbursement-key")
		writeJSON(t, w, map[string]any{
			"status": "FAILED",
			"reason": map[string]any{"code": "NOT_ENOUGH_FUNDS", "message": "insufficient funds"},
		})
	})
	mux.HandleFunc("GET /disbursement/v1_0/account/balance", func(w http.ResponseWriter, r *http.Request) {
		assertAPIHeaders(t, r, "disbursement-token", "disbursement-key")
		writeJSON(t, w, map[string]any{"availableBalance": "42.50", "currency": "EUR"})
	})
	provider := testProvider(t, map[string]string{
		"base_url":                      "https://mtn.test",
		"disbursement_subscription_key": "disbursement-key",
		"balance_service":               "disbursement",
		"transfer_type":                 "CUSTOM_PAYMENT",
	})
	provider.client = &http.Client{Transport: handlerTransport{handler: mux}}
	ctx := context.Background()
	if err := provider.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	collection, err := provider.Collect(ctx, payment("collect"))
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	disbursement, err := provider.Disburse(ctx, payment("disburse"))
	if err != nil {
		t.Fatalf("Disburse() error = %v", err)
	}
	assertSubmittedPayment(t, requests[domain.ServiceCollection], collection.ProviderReference, true)
	assertSubmittedPayment(t, requests[domain.ServiceDisbursement], disbursement.ProviderReference, false)

	collectionStatus, err := provider.QueryTransaction(ctx, collection.ProviderReference, "UG")
	if err != nil {
		t.Fatalf("QueryTransaction(collection) error = %v", err)
	}
	if collectionStatus.Status != domain.TxSucceeded {
		t.Errorf("collection status = %q, want succeeded", collectionStatus.Status)
	}
	disbursementStatus, err := provider.QueryTransaction(ctx, disbursement.ProviderReference, "UG")
	if err != nil {
		t.Fatalf("QueryTransaction(disbursement) error = %v", err)
	}
	if disbursementStatus.Status != domain.TxFailed || disbursementStatus.Message != "insufficient funds" {
		t.Errorf("disbursement status = %#v, want failed with reason", disbursementStatus)
	}
	balance, err := provider.QueryBalance(ctx, "UG")
	if err != nil {
		t.Fatalf("QueryBalance() error = %v", err)
	}
	if balance.Currency != "EUR" || balance.Available != 4250 || balance.Ledger != 4250 {
		t.Errorf("QueryBalance() = %#v, want EUR 4250", balance)
	}
	if tokenCalls[domain.ServiceCollection] != 1 || tokenCalls[domain.ServiceDisbursement] != 1 {
		t.Errorf("token calls = %v, want one call per product", tokenCalls)
	}
}

func TestQueryTransactionRejectsMalformedReference(t *testing.T) {
	provider := testProvider(t, nil)
	for _, reference := range []string{"missing-service", "unknown:" + uuid.NewString(), "collection:not-a-uuid"} {
		if _, err := provider.QueryTransaction(context.Background(), reference, "UG"); err == nil {
			t.Errorf("QueryTransaction(%q) error = nil, want an error", reference)
		}
	}
}

func TestVerifyWebhookNormalizesCollectionAndDisbursement(t *testing.T) {
	provider := testProvider(t, nil)
	for _, test := range []struct {
		service string
		party   string
	}{
		{service: domain.ServiceCollection, party: "payer"},
		{service: domain.ServiceDisbursement, party: "payee"},
	} {
		t.Run(test.service, func(t *testing.T) {
			id := uuid.NewString()
			body := map[string]any{
				"externalId": id,
				"amount":     "10.25",
				"currency":   "eur",
				"status":     "SUCCESSFUL",
				test.party: map[string]any{
					"partyIdType": "MSISDN",
					"partyId":     "+256 770 000 000",
				},
			}
			payload, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			event, err := provider.VerifyWebhook(context.Background(), payload, nil)
			if err != nil {
				t.Fatalf("VerifyWebhook() error = %v", err)
			}
			if event.ProviderReference != makeReference(test.service, id) || event.Status != domain.TxSucceeded {
				t.Errorf("VerifyWebhook() reference/status = %q/%q", event.ProviderReference, event.Status)
			}
			if event.Amount == nil || *event.Amount != 1025 || event.Currency != "EUR" || event.Account != "256770000000" {
				t.Errorf("VerifyWebhook() payment fields = %#v", event)
			}
		})
	}

	invalid := []string{
		`{"externalId":"bad","payer":{"partyId":"256770000000"}}`,
		`{"externalId":"` + uuid.NewString() + `"}`,
	}
	for _, payload := range invalid {
		if _, err := provider.VerifyWebhook(context.Background(), []byte(payload), nil); err == nil {
			t.Errorf("VerifyWebhook(%s) error = nil, want an error", payload)
		}
	}
}

func assertTokenRequest(t *testing.T, r *http.Request, subscriptionKey string) {
	t.Helper()
	assertTokenRequestCredentials(t, r, subscriptionKey, "api-user:api-key")
}

func assertTokenRequestCredentials(
	t *testing.T,
	r *http.Request,
	subscriptionKey string,
	credentials string,
) {
	t.Helper()
	wantAuthorization := "Basic " + base64.StdEncoding.EncodeToString([]byte(credentials))
	if got := r.Header.Get("Authorization"); got != wantAuthorization {
		t.Errorf("Authorization = %q, want Basic credentials", got)
	}
	if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != subscriptionKey {
		t.Errorf("subscription key = %q, want %q", got, subscriptionKey)
	}
}

func decodePayment(t *testing.T, r *http.Request, token, subscriptionKey string) paymentRequest {
	t.Helper()
	assertAPIHeaders(t, r, token, subscriptionKey)
	var body paymentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if body.ExternalID != r.Header.Get("X-Reference-Id") {
		t.Errorf("externalId = %q, want X-Reference-Id %q", body.ExternalID, r.Header.Get("X-Reference-Id"))
	}
	if id, err := uuid.Parse(body.ExternalID); err != nil || id.Version() != 4 {
		t.Errorf("externalId = %q, want a version 4 UUID", body.ExternalID)
	}
	return body
}

func assertAPIHeaders(t *testing.T, r *http.Request, token, subscriptionKey string) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+token {
		t.Errorf("Authorization = %q, want bearer token", got)
	}
	if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != subscriptionKey {
		t.Errorf("subscription key = %q, want %q", got, subscriptionKey)
	}
	if got := r.Header.Get("X-Target-Environment"); got != "mtnuganda" {
		t.Errorf("X-Target-Environment = %q, want mtnuganda", got)
	}
}

func assertSubmittedPayment(t *testing.T, body paymentRequest, reference string, collection bool) {
	t.Helper()
	_, id, err := parseReference(reference)
	if err != nil {
		t.Fatalf("parseReference(%q) error = %v", reference, err)
	}
	if body.ExternalID != id || body.Amount != "12.50" || body.Currency != "EUR" {
		t.Errorf("submitted payment = %#v, want matching reference and EUR 12.50", body)
	}
	if body.TransferType != "CUSTOM_PAYMENT" {
		t.Errorf("TransferType = %q, want CUSTOM_PAYMENT", body.TransferType)
	}
	if collection && (body.Payer == nil || body.Payee != nil) {
		t.Errorf("collection party = payer %#v, payee %#v", body.Payer, body.Payee)
	}
	if !collection && (body.Payee == nil || body.Payer != nil) {
		t.Errorf("disbursement party = payer %#v, payee %#v", body.Payer, body.Payee)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("Encode() error = %v", err)
	}
}

func mergeConfig(base providers.ProviderConfig, overrides map[string]string) providers.ProviderConfig {
	merged := maps.Clone(base)
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

func TestNarrativeIsBounded(t *testing.T) {
	req := payment("narrative")
	req.Description = strings.Repeat("y", 161)
	if got := narrative(req); len([]rune(got)) != 160 {
		t.Errorf("narrative length = %d, want 160", len([]rune(got)))
	}
}
