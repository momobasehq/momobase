package dummy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// SignatureHeader carries the hexadecimal HMAC-SHA256 signature of a webhook body.
const SignatureHeader = "X-Momobase-Signature"

// The outcomes a payment can be configured to reach.
const (
	// OutcomeSucceed settles payments successfully.
	OutcomeSucceed = "succeed"
	// OutcomeFail settles payments as permanently failed.
	OutcomeFail = "fail"
	// OutcomePending leaves payments pending indefinitely, simulating a stuck payment.
	OutcomePending = "pending"
	// OutcomeUnknown leaves payments in the unknown state, simulating an unclear provider result.
	OutcomeUnknown = "unknown"
	// OutcomeError makes payment requests fail before a payment is created, simulating a transport error.
	OutcomeError = "error"
)

const (
	defaultCurrency = "UGX"
	defaultBalance  = 1_000_000
)

// Config contains the settings recognized in a dummy provider account's
// configuration. Every field is optional except the webhook signing credential,
// which is read from the "webhook_secret" key.
type Config struct {
	// Outcome selects the result payments reach: OutcomeSucceed, OutcomeFail,
	// OutcomePending, OutcomeUnknown, or OutcomeError. It defaults to
	// OutcomeSucceed and is read from "outcome".
	Outcome string
	// SettleAfter is the number of status queries after which a payment reaches
	// Outcome. Zero settles a payment immediately, one settles it on the first
	// query, and so on. It is read from "settle_after".
	SettleAfter int
	// LatencyMs delays every operation so that timeout and cancellation handling
	// can be exercised. It is read from "latency_ms".
	LatencyMs int
	// FailInit makes Init fail, simulating invalid credentials. It is read from "fail_init".
	FailInit bool
	// FailHealth makes HealthCheck fail, simulating an unreachable upstream. It is
	// read from "fail_health".
	FailHealth bool
	// Services lists the service types reported as capabilities, as a
	// comma-separated list of collection and disbursement. It defaults to both and
	// is read from "services".
	Services []string
	// Currency is the currency reported by balance queries. It defaults to UGX and
	// is read from "currency".
	Currency string
	// Balance is the opening balance in minor units. Collections credit it and
	// disbursements debit it. It defaults to 1000000 and is read from "balance".
	Balance int64
	// WebhookSecret authenticates and signs webhook payloads. It is required and
	// is read from "webhook_secret".
	WebhookSecret string
}

// record is the simulated state of one payment in the provider's ledger.
type record struct {
	service   string
	amount    int64
	currency  string
	status    string
	final     string
	remaining int
	applied   bool
}

// Provider simulates a payment provider using an in-memory ledger. It performs
// no network I/O and moves no money.
type Provider struct {
	log *slog.Logger

	mu      sync.RWMutex
	cfg     Config
	balance int64
	records map[string]*record
}

// New returns a dummy provider ready to be initialized with configuration.
func New(log *slog.Logger) providers.PaymentProvider {
	return &Provider{log: log, records: map[string]*record{}}
}

// Reference returns the provider reference the dummy provider assigns to a
// transaction. It is deterministic so that callers can construct a webhook body
// for a payment without reading the response that created it.
func Reference(transactionID string) string {
	return "dummy_" + transactionID
}

// Sign returns the hexadecimal HMAC-SHA256 signature of a webhook payload, which
// the provider expects in the SignatureHeader header.
func Sign(credential string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(credential))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Capabilities returns the configured identity.
func (p *Provider) Capabilities() []providers.Capability {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]providers.Capability, 0, len(p.cfg.Services))
	for _, service := range p.cfg.Services {
		out = append(out, providers.Capability{ServiceType: service})
	}
	return out
}

// Init validates and applies the provider configuration and resets the ledger.
func (p *Provider) Init(_ context.Context, raw providers.ProviderConfig) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if cfg.FailInit {
		return errors.New("dummy: initialization failed by configuration")
	}
	p.mu.Lock()
	p.cfg, p.balance, p.records = cfg, cfg.Balance, map[string]*record{}
	p.mu.Unlock()
	p.log.Warn("dummy provider initialized: it simulates payments and moves no money")
	return nil
}

// HealthCheck reports the health configured for the provider.
func (p *Provider) HealthCheck(ctx context.Context) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	p.mu.RLock()
	ready, fail := p.cfg.WebhookSecret != "", p.cfg.FailHealth
	p.mu.RUnlock()
	if !ready {
		return errors.New("dummy: provider is not initialized")
	}
	if fail {
		return errors.New("dummy: upstream is unavailable by configuration")
	}
	return nil
}

// Collect simulates collecting a payment from a customer.
func (p *Provider) Collect(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.pay(ctx, domain.ServiceCollection, req)
}

// Disburse simulates disbursing a payment to a recipient.
func (p *Provider) Disburse(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.pay(ctx, domain.ServiceDisbursement, req)
}

func (p *Provider) pay(ctx context.Context, service string, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cfg.WebhookSecret == "" {
		return nil, errors.New("dummy: provider is not initialized")
	}
	if !supports(p.cfg.Services, service) {
		return nil, fmt.Errorf("dummy: %s is not enabled for this account", service)
	}
	if p.cfg.Outcome == OutcomeError {
		return nil, errors.New("dummy: upstream rejected the request by configuration")
	}
	reference := Reference(req.TransactionID)
	entry := &record{
		service:   service,
		amount:    req.Amount,
		currency:  utils.First(req.Currency, p.cfg.Currency),
		final:     settled(p.cfg.Outcome),
		remaining: p.cfg.SettleAfter,
	}
	entry.status = entry.final
	if entry.remaining > 0 {
		entry.status = domain.TxProcessing
	}
	p.records[reference] = entry
	p.apply(entry)
	return &providers.ProviderPaymentResponse{
		ProviderReference: reference,
		Status:            entry.status,
		Message:           "simulated: " + entry.status,
		Raw:               map[string]any{"simulated": true, "reference": reference, "status": entry.status},
	}, nil
}

// QueryTransaction returns the simulated state of a payment, advancing it toward
// its configured outcome by one step.
func (p *Provider) QueryTransaction(ctx context.Context, reference, _ string) (*providers.ProviderTransactionStatus, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.records[reference]
	if !ok {
		return nil, fmt.Errorf("dummy: no simulated payment for reference %q", reference)
	}
	if entry.remaining > 0 {
		entry.remaining--
		if entry.remaining == 0 {
			entry.status = entry.final
			p.apply(entry)
		}
	}
	return &providers.ProviderTransactionStatus{
		ProviderReference: reference,
		Status:            entry.status,
		Message:           "simulated: " + entry.status,
	}, nil
}

// QueryBalance returns the simulated account balance.
func (p *Provider) QueryBalance(ctx context.Context, _ string) (*providers.ProviderBalance, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cfg.WebhookSecret == "" {
		return nil, errors.New("dummy: provider is not initialized")
	}
	return &providers.ProviderBalance{Currency: p.cfg.Currency, Available: p.balance, Ledger: p.balance}, nil
}

// webhookPayload is the body the dummy provider accepts on its webhook endpoint.
type webhookPayload struct {
	Reference         string `json:"reference"`
	Status            string `json:"status"`
	EventType         string `json:"event_type"`
	Amount            string `json:"amount"`
	Currency          string `json:"currency"`
	Country           string `json:"country"`
	ExternalReference string `json:"external_reference"`
	Account           string `json:"account"`
}

// VerifyWebhook authenticates a webhook payload against the configured signing
// credential and applies its status to the ledger.
func (p *Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*providers.ProviderWebhookEvent, error) {
	p.mu.RLock()
	credential := p.cfg.WebhookSecret
	p.mu.RUnlock()
	if credential == "" {
		return nil, errors.New("dummy: provider is not initialized")
	}
	expected := Sign(credential, payload)
	if !hmac.Equal([]byte(header(headers, SignatureHeader)), []byte(expected)) {
		return nil, errors.New("dummy: webhook signature mismatch")
	}
	var body webhookPayload
	// The decoder error is deliberately not wrapped: it can quote payload bytes,
	// and provider errors reach operators and API clients.
	if json.Unmarshal(payload, &body) != nil {
		return nil, errors.New("dummy: webhook payload is not valid JSON")
	}
	if strings.TrimSpace(body.Reference) == "" {
		return nil, errors.New("dummy: webhook payload has no reference")
	}
	status := providers.PaymentStatus(body.Status)
	currency := strings.ToUpper(utils.First(body.Currency, p.currency(body.Reference)))
	amount, err := providers.OptionalAmount(body.Amount, currency)
	if err != nil {
		return nil, errors.New("dummy: webhook payload has an unreadable amount")
	}
	p.settleFromWebhook(body.Reference, status)
	// The account is passed through as the payload spells it. The dummy provider
	// normalizes nothing, so an account it reports matches the transaction only when
	// the caller sends the same value it paid with, which is what makes the engine's
	// match check testable from a webhook body.
	return &providers.ProviderWebhookEvent{
		ProviderReference: body.Reference,
		Status:            status,
		EventType:         utils.First(body.EventType, "payment.updated"),
		ExternalReference: body.ExternalReference,
		Amount:            amount,
		Currency:          body.Currency,
		Country:           body.Country,
		Account:           strings.TrimSpace(body.Account),
		Raw:               map[string]any{"simulated": true, "reference": body.Reference, "status": status},
	}, nil
}

func (p *Provider) currency(reference string) string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if entry, ok := p.records[reference]; ok {
		return entry.currency
	}
	return p.cfg.Currency
}

func (p *Provider) settleFromWebhook(reference, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.records[reference]
	if !ok {
		return
	}
	entry.status, entry.remaining = status, 0
	p.apply(entry)
}

// apply adjusts the simulated balance once, when a payment first succeeds.
// It must be called with the write lock held.
func (p *Provider) apply(entry *record) {
	if entry.applied || entry.status != domain.TxSucceeded {
		return
	}
	entry.applied = true
	if entry.service == domain.ServiceDisbursement {
		if p.balance -= entry.amount; p.balance < 0 {
			p.balance = 0
		}
		return
	}
	p.balance += entry.amount
}

// wait applies the configured latency and reports cancellation.
func (p *Provider) wait(ctx context.Context) error {
	p.mu.RLock()
	delay := time.Duration(p.cfg.LatencyMs) * time.Millisecond
	p.mu.RUnlock()
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// settled maps a configured outcome onto the transaction status it reaches.
func settled(outcome string) string {
	switch outcome {
	case OutcomeFail:
		return domain.TxFailed
	case OutcomePending:
		return domain.TxPending
	case OutcomeUnknown:
		return domain.TxUnknown
	default:
		return domain.TxSucceeded
	}
}

func supports(services []string, service string) bool {
	for _, value := range services {
		if value == service {
			return true
		}
	}
	return false
}

// header reads a header case-insensitively, since callers normalize differently.
func header(headers map[string]string, name string) string {
	if value, ok := headers[name]; ok {
		return value
	}
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func parseConfig(raw providers.ProviderConfig) (Config, error) {
	cfg := Config{
		Outcome:       strings.ToLower(utils.First(utils.String(raw, "outcome"), OutcomeSucceed)),
		SettleAfter:   utils.Int(raw, "settle_after"),
		LatencyMs:     utils.Int(raw, "latency_ms"),
		FailInit:      utils.Bool(raw, "fail_init"),
		FailHealth:    utils.Bool(raw, "fail_health"),
		Services:      parseServices(utils.String(raw, "services")),
		Currency:      strings.ToUpper(utils.First(utils.String(raw, "currency"), defaultCurrency)),
		Balance:       defaultBalance,
		WebhookSecret: utils.String(raw, "webhook_secret"),
	}
	if utils.String(raw, "balance") != "" {
		cfg.Balance = int64(utils.Int(raw, "balance"))
	}
	if cfg.WebhookSecret == "" {
		return Config{}, errors.New("dummy: a webhook signing credential is required")
	}
	switch cfg.Outcome {
	case OutcomeSucceed, OutcomeFail, OutcomePending, OutcomeUnknown, OutcomeError:
	default:
		return Config{}, fmt.Errorf("dummy: unsupported outcome %q", cfg.Outcome)
	}
	if cfg.SettleAfter < 0 || cfg.LatencyMs < 0 {
		return Config{}, errors.New("dummy: settle_after and latency_ms must not be negative")
	}
	if len(cfg.Services) == 0 {
		return Config{}, errors.New("dummy: at least one service is required")
	}
	for _, service := range cfg.Services {
		if service != domain.ServiceCollection && service != domain.ServiceDisbursement {
			return Config{}, fmt.Errorf("dummy: unsupported service %q", service)
		}
	}
	return cfg, nil
}

func parseServices(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{domain.ServiceCollection, domain.ServiceDisbursement}
	}
	out := make([]string, 0, 2)
	for _, value := range strings.Split(raw, ",") {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			out = append(out, value)
		}
	}
	return out
}
