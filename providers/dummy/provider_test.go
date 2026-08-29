package dummy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

const credential = "signing-credential-0123456789"

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testConfig(options map[string]string) providers.ProviderConfig {
	config := providers.ProviderConfig{"webhook_secret": credential}
	for key, value := range options {
		config[key] = value
	}
	return config
}

// newProvider builds an initialized provider from the supplied options.
func newProvider(t *testing.T, options map[string]string) *Provider {
	t.Helper()
	provider, ok := New(testLogger()).(*Provider)
	if !ok {
		t.Fatal("New() did not return *Provider")
	}
	if err := provider.Init(context.Background(), testConfig(options)); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return provider
}

func request(id string, amount int64) providers.PaymentRequest {
	return providers.PaymentRequest{
		TransactionID: id,
		Amount:        amount,
		Currency:      "UGX",
		Country:       "UG",
		Reference:     "ref-" + id,
		Account:       "256770000000",
	}
}

func TestParseConfigDefaultsAndOverrides(t *testing.T) {
	provider := newProvider(t, nil)
	if got := provider.cfg.Outcome; got != OutcomeSucceed {
		t.Errorf("Outcome = %q, want %q", got, OutcomeSucceed)
	}
	if got := provider.cfg.Currency; got != defaultCurrency {
		t.Errorf("Currency = %q, want %q", got, defaultCurrency)
	}
	if got := provider.cfg.Balance; got != defaultBalance {
		t.Errorf("Balance = %d, want %d", got, defaultBalance)
	}
	if got := len(provider.cfg.Services); got != 2 {
		t.Errorf("Services = %v, want both services", provider.cfg.Services)
	}

	provider = newProvider(t, map[string]string{
		"outcome":      " FAIL ",
		"currency":     "kes",
		"balance":      "250",
		"services":     " collection , ",
		"settle_after": "3",
	})
	if got := provider.cfg.Outcome; got != OutcomeFail {
		t.Errorf("Outcome = %q, want %q", got, OutcomeFail)
	}
	if got := provider.cfg.Currency; got != "KES" {
		t.Errorf("Currency = %q, want KES", got)
	}
	if got := provider.cfg.Balance; got != 250 {
		t.Errorf("Balance = %d, want 250", got)
	}
	if got := provider.cfg.Services; len(got) != 1 || got[0] != domain.ServiceCollection {
		t.Errorf("Services = %v, want [collection]", got)
	}
	if got := provider.cfg.SettleAfter; got != 3 {
		t.Errorf("SettleAfter = %d, want 3", got)
	}
}

func TestParseConfigRejectsInvalidValues(t *testing.T) {
	tests := map[string]providers.ProviderConfig{
		"missing credential": {},
		"unknown outcome":    testConfig(map[string]string{"outcome": "explode"}),
		"unknown service":    testConfig(map[string]string{"services": "collection,cards"}),
		"negative settle":    testConfig(map[string]string{"settle_after": "-1"}),
		"negative latency":   testConfig(map[string]string{"latency_ms": "-5"}),
	}
	for name, config := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(config); err == nil {
				t.Fatalf("parseConfig(%v) error = nil, want an error", config)
			}
		})
	}
}

func TestInitFailureAndCapabilities(t *testing.T) {
	provider, ok := New(nil).(*Provider)
	if !ok {
		t.Fatal("New() did not return *Provider")
	}
	if got := provider.Capabilities(); len(got) != 0 {
		t.Errorf("Capabilities() before Init = %v, want none", got)
	}
	if err := provider.HealthCheck(context.Background()); err == nil {
		t.Error("HealthCheck() before Init = nil, want an error")
	}

	provider = &Provider{log: testLogger(), records: map[string]*record{}}
	err := provider.Init(context.Background(), testConfig(map[string]string{"fail_init": "true"}))
	if err == nil {
		t.Fatal("Init(fail_init) error = nil, want an error")
	}

	provider = newProvider(t, map[string]string{"services": "disbursement"})
	caps := provider.Capabilities()
	if len(caps) != len(providers.PaymentMethods()) || caps[0].ServiceType != domain.ServiceDisbursement {
		t.Fatalf("Capabilities() = %v, want disbursement capabilities", caps)
	}
	if !providers.SupportsService(caps, domain.ServiceDisbursement) || providers.SupportsService(caps, domain.ServiceCollection) {
		t.Error("Supports() did not report a disbursement-only account")
	}
	if _, err = provider.Collect(context.Background(), request("txn_1", 100)); err == nil {
		t.Error("Collect() on a disbursement-only account = nil, want an error")
	}
}

func TestHealthCheckReportsConfiguredFailure(t *testing.T) {
	if err := newProvider(t, nil).HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	err := newProvider(t, map[string]string{"fail_health": "true"}).HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck(fail_health) error = nil, want an error")
	}
}

func TestPaymentOutcomes(t *testing.T) {
	tests := []struct {
		outcome string
		want    string
	}{
		{OutcomeSucceed, domain.TxSucceeded},
		{OutcomeFail, domain.TxFailed},
		{OutcomePending, domain.TxPending},
		{OutcomeUnknown, domain.TxUnknown},
	}
	for _, test := range tests {
		t.Run(test.outcome, func(t *testing.T) {
			provider := newProvider(t, map[string]string{"outcome": test.outcome})
			for _, service := range []string{domain.ServiceCollection, domain.ServiceDisbursement} {
				result, err := provider.pay(context.Background(), service, request("txn_"+service, 500))
				if err != nil {
					t.Fatalf("pay(%s) error = %v", service, err)
				}
				if result.Status != test.want {
					t.Errorf("pay(%s) status = %q, want %q", service, result.Status, test.want)
				}
				if result.ProviderReference != Reference("txn_"+service) {
					t.Errorf("pay(%s) reference = %q, want the deterministic reference", service, result.ProviderReference)
				}
				if result.Raw["simulated"] != true {
					t.Errorf("pay(%s) raw = %v, want a simulated marker", service, result.Raw)
				}
			}
		})
	}

	provider := newProvider(t, map[string]string{"outcome": OutcomeError})
	if _, err := provider.Collect(context.Background(), request("txn_err", 100)); err == nil {
		t.Fatal("Collect(outcome=error) error = nil, want an error")
	}
	if _, ok := provider.records[Reference("txn_err")]; ok {
		t.Error("Collect(outcome=error) recorded a payment, want none")
	}
}

func TestQueryTransactionSettlesAfterConfiguredQueries(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t, map[string]string{"settle_after": "2"})

	result, err := provider.Collect(ctx, request("txn_settle", 700))
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if result.Status != domain.TxProcessing {
		t.Fatalf("Collect() status = %q, want %q", result.Status, domain.TxProcessing)
	}
	reference := result.ProviderReference

	for i, want := range []string{domain.TxProcessing, domain.TxSucceeded, domain.TxSucceeded} {
		status, queryErr := provider.QueryTransaction(ctx, reference, "UG")
		if queryErr != nil {
			t.Fatalf("QueryTransaction() call %d error = %v", i+1, queryErr)
		}
		if status.Status != want {
			t.Fatalf("QueryTransaction() call %d = %q, want %q", i+1, status.Status, want)
		}
	}

	if _, err = provider.QueryTransaction(ctx, "dummy_missing", "UG"); err == nil {
		t.Error("QueryTransaction(unknown) error = nil, want an error")
	}
}

func TestBalanceTracksSettledPayments(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t, map[string]string{"balance": "1000"})

	balance, err := provider.QueryBalance(ctx, "UG")
	if err != nil {
		t.Fatalf("QueryBalance() error = %v", err)
	}
	if balance.Available != 1000 || balance.Currency != defaultCurrency {
		t.Fatalf("QueryBalance() = %+v, want 1000 %s", balance, defaultCurrency)
	}

	if _, err = provider.Collect(ctx, request("txn_in", 250)); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if _, err = provider.Disburse(ctx, request("txn_out", 100)); err != nil {
		t.Fatalf("Disburse() error = %v", err)
	}
	if balance, err = provider.QueryBalance(ctx, "UG"); err != nil {
		t.Fatalf("QueryBalance() error = %v", err)
	}
	if balance.Available != 1150 {
		t.Fatalf("QueryBalance() available = %d, want 1150", balance.Available)
	}

	// A disbursement larger than the balance clamps at zero rather than going negative.
	provider = newProvider(t, map[string]string{"balance": "50"})
	if _, err = provider.Disburse(ctx, request("txn_big", 500)); err != nil {
		t.Fatalf("Disburse() error = %v", err)
	}
	if balance, err = provider.QueryBalance(ctx, "UG"); err != nil {
		t.Fatalf("QueryBalance() error = %v", err)
	}
	if balance.Available != 0 {
		t.Fatalf("QueryBalance() available = %d, want 0", balance.Available)
	}
}

// signedWebhook marshals a payload and returns it with a valid signature header.
func signedWebhook(t *testing.T, body map[string]any) ([]byte, map[string]string) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal webhook: %v", err)
	}
	return payload, map[string]string{SignatureHeader: Sign(credential, payload)}
}

func TestVerifyWebhookAuthenticatesAndAppliesStatus(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t, map[string]string{"settle_after": "5"})
	result, err := provider.Collect(ctx, request("txn_hook", 400))
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	payload, headers := signedWebhook(t, map[string]any{
		"reference":          result.ProviderReference,
		"status":             "SUCCESSFUL",
		"amount":             "4",
		"currency":           "ugx",
		"external_reference": "ref-txn_hook",
	})
	event, err := provider.VerifyWebhook(ctx, payload, headers)
	if err != nil {
		t.Fatalf("VerifyWebhook() error = %v", err)
	}
	if event.Status != domain.TxSucceeded {
		t.Errorf("VerifyWebhook() status = %q, want %q", event.Status, domain.TxSucceeded)
	}
	if event.Amount == nil || *event.Amount != 4 {
		t.Errorf("VerifyWebhook() amount = %v, want 4 minor units", event.Amount)
	}
	if event.EventType != "payment.updated" {
		t.Errorf("VerifyWebhook() event type = %q, want the default", event.EventType)
	}

	// The webhook settles the ledger, so the pending countdown is abandoned.
	status, err := provider.QueryTransaction(ctx, result.ProviderReference, "UG")
	if err != nil {
		t.Fatalf("QueryTransaction() error = %v", err)
	}
	if status.Status != domain.TxSucceeded {
		t.Errorf("QueryTransaction() after webhook = %q, want %q", status.Status, domain.TxSucceeded)
	}

	// A webhook for an unknown reference is still a valid event.
	payload, headers = signedWebhook(t, map[string]any{"reference": "dummy_absent", "status": "FAILED"})
	if _, err = provider.VerifyWebhook(ctx, payload, headers); err != nil {
		t.Errorf("VerifyWebhook(unknown reference) error = %v", err)
	}
}

func TestVerifyWebhookRejectsBadInput(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t, nil)
	payload, headers := signedWebhook(t, map[string]any{"reference": "dummy_1", "status": "SUCCESSFUL"})

	tests := map[string]struct {
		payload []byte
		headers map[string]string
	}{
		"missing signature": {payload, map[string]string{}},
		"wrong signature":   {payload, map[string]string{SignatureHeader: Sign("other-credential", payload)}},
		"tampered body":     {append(payload, ' '), headers},
		"invalid json":      {[]byte("{nope"), map[string]string{SignatureHeader: Sign(credential, []byte("{nope"))}},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := provider.VerifyWebhook(ctx, test.payload, test.headers); err == nil {
				t.Fatal("VerifyWebhook() error = nil, want an error")
			}
		})
	}

	empty, emptyHeaders := signedWebhook(t, map[string]any{"status": "SUCCESSFUL"})
	if _, err := provider.VerifyWebhook(ctx, empty, emptyHeaders); err == nil {
		t.Error("VerifyWebhook(no reference) error = nil, want an error")
	}

	bad, badHeaders := signedWebhook(t, map[string]any{"reference": "dummy_1", "amount": "1.234", "currency": "USD"})
	if _, err := provider.VerifyWebhook(ctx, bad, badHeaders); err == nil {
		t.Error("VerifyWebhook(bad amount) error = nil, want an error")
	}
}

func TestSignatureHeaderIsCaseInsensitive(t *testing.T) {
	provider := newProvider(t, nil)
	payload, _ := signedWebhook(t, map[string]any{"reference": "dummy_1", "status": "SUCCESSFUL"})
	headers := map[string]string{strings.ToLower(SignatureHeader): Sign(credential, payload)}
	if _, err := provider.VerifyWebhook(context.Background(), payload, headers); err != nil {
		t.Fatalf("VerifyWebhook() with a lowercase header error = %v", err)
	}
}

func TestLatencyRespectsCancellation(t *testing.T) {
	provider := newProvider(t, map[string]string{"latency_ms": "2000"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := provider.Collect(ctx, request("txn_slow", 100)); !errors.Is(err, context.Canceled) {
		t.Errorf("Collect() error = %v, want context.Canceled", err)
	}
	if err := provider.HealthCheck(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("HealthCheck() error = %v, want context.Canceled", err)
	}
	if _, err := provider.QueryBalance(ctx, "UG"); !errors.Is(err, context.Canceled) {
		t.Errorf("QueryBalance() error = %v, want context.Canceled", err)
	}

	// A short latency still completes against a live context.
	provider = newProvider(t, map[string]string{"latency_ms": "1"})
	timed, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if _, err := provider.Collect(timed, request("txn_ok", 100)); err != nil {
		t.Errorf("Collect() error = %v", err)
	}
}

// TestErrorsSurviveRedaction guards every provider-authored error against
// providers.Redact, which blanks any message containing a credential-like word.
// A redacted message would reach operators as "[redacted provider error]".
func TestErrorsSurviveRedaction(t *testing.T) {
	ctx := context.Background()
	uninitialized := &Provider{log: testLogger(), records: map[string]*record{}}
	limited := newProvider(t, map[string]string{"services": "collection"})
	failing := newProvider(t, map[string]string{"outcome": OutcomeError})
	healthy := newProvider(t, nil)
	payload, _ := signedWebhook(t, map[string]any{"reference": "dummy_1"})

	_, missingCredential := parseConfig(providers.ProviderConfig{})
	_, badOutcome := parseConfig(testConfig(map[string]string{"outcome": "explode"}))
	_, badService := parseConfig(testConfig(map[string]string{"services": "cards"}))
	_, negative := parseConfig(testConfig(map[string]string{"settle_after": "-1"}))
	initFailed := uninitialized.Init(ctx, testConfig(map[string]string{"fail_init": "true"}))
	_, unsupported := limited.Disburse(ctx, request("txn_1", 100))
	_, transport := failing.Collect(ctx, request("txn_2", 100))
	_, unknownReference := healthy.QueryTransaction(ctx, "dummy_absent", "UG")
	_, badSignature := healthy.VerifyWebhook(ctx, payload, map[string]string{SignatureHeader: "deadbeef"})
	_, badJSON := healthy.VerifyWebhook(ctx, []byte("{nope"), map[string]string{SignatureHeader: Sign(credential, []byte("{nope"))})
	noReference := func() error {
		body, headers := signedWebhook(t, map[string]any{"status": "SUCCESSFUL"})
		_, err := healthy.VerifyWebhook(ctx, body, headers)
		return err
	}()
	_, notInitialized := uninitialized.QueryBalance(ctx, "UG")
	unhealthy := newProvider(t, map[string]string{"fail_health": "true"}).HealthCheck(ctx)

	errs := map[string]error{
		"missing credential": missingCredential,
		"bad outcome":        badOutcome,
		"bad service":        badService,
		"negative value":     negative,
		"init failed":        initFailed,
		"unsupported":        unsupported,
		"transport":          transport,
		"unknown reference":  unknownReference,
		"bad signature":      badSignature,
		"invalid json":       badJSON,
		"no reference":       noReference,
		"not initialized":    notInitialized,
		"unhealthy":          unhealthy,
	}
	for name, err := range errs {
		t.Run(name, func(t *testing.T) {
			if err == nil {
				t.Fatal("error = nil, want an error to check")
			}
			if got := providers.Redact(err.Error()); got != err.Error() {
				t.Fatalf("Redact(%q) = %q; the message must avoid credential-like words", err.Error(), got)
			}
		})
	}
}

func TestConcurrentUseIsRaceFree(t *testing.T) {
	ctx := context.Background()
	provider := newProvider(t, map[string]string{"settle_after": "1"})

	var wait sync.WaitGroup
	for i := range 16 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			id := "txn_" + string(rune('a'+i%16))
			if _, err := provider.Collect(ctx, request(id, 10)); err != nil {
				t.Errorf("Collect() error = %v", err)
				return
			}
			if _, err := provider.QueryTransaction(ctx, Reference(id), "UG"); err != nil {
				t.Errorf("QueryTransaction() error = %v", err)
			}
			provider.Capabilities()
			if _, err := provider.QueryBalance(ctx, "UG"); err != nil {
				t.Errorf("QueryBalance() error = %v", err)
			}
		}()
	}
	wait.Wait()
}
