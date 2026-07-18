package mtn

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/providers"
)

const baseURL = "https://sandbox.momodeveloper.mtn.com"

type product struct {
	name   string
	key    string
	user   string
	secret string
}
type Config struct {
	BaseURL           string
	TargetEnvironment string
	Currency          string
	CallbackURL       string
	Collection        product
	Disbursement      product
	Timeout           time.Duration
}
type Provider struct {
	client *http.Client
	cfg    Config
	tokens map[string]*providers.TokenCache
}

func New(*slog.Logger) providers.PaymentProvider {
	return &Provider{}
}
func (p *Provider) Capabilities() []providers.Capability {
	out := make([]providers.Capability, 0, 2)
	if p.cfg.Collection.valid() {
		out = append(out, providers.Capability{ServiceType: domain.ServiceCollection, PaymentMethod: domain.PaymentMethodMomo})
	}
	if p.cfg.Disbursement.valid() {
		out = append(out, providers.Capability{ServiceType: domain.ServiceDisbursement, PaymentMethod: domain.PaymentMethodMomo})
	}
	return out
}
func (p *Provider) Init(_ context.Context, raw providers.ProviderConfig) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	p.cfg, p.client = cfg, &http.Client{Timeout: cfg.Timeout}
	p.tokens = map[string]*providers.TokenCache{"collection": {}, "disbursement": {}}
	return nil
}
func (p *Provider) HealthCheck(ctx context.Context) error {
	if len(p.Capabilities()) == 0 {
		return errors.New("mtn has no complete credential set")
	}
	for _, item := range []product{p.cfg.Collection, p.cfg.Disbursement} {
		if item.valid() {
			if _, err := p.token(ctx, item); err != nil {
				return fmt.Errorf("mtn %s health: %w", item.name, err)
			}
		}
	}
	return nil
}
func (p *Provider) Collect(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.payment(ctx, p.cfg.Collection, "/collection/v1_0/requesttopay", "payer", req)
}
func (p *Provider) Disburse(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.payment(ctx, p.cfg.Disbursement, "/disbursement/v1_0/transfer", "payee", req)
}
func (p *Provider) payment(
	ctx context.Context,
	item product,
	path string,
	party string,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	if !item.valid() {
		return nil, fmt.Errorf("mtn %s credentials are incomplete", item.name)
	}
	currency, ref := providers.First(req.Currency, p.cfg.Currency), providers.UUID()
	note := req.Description
	if len(note) > 160 {
		note = note[:160]
	}
	body := map[string]any{
		"amount":     providers.FormatAmountMinor(req.Amount, currency),
		"currency":   currency,
		"externalId": req.Reference,
		party: map[string]string{
			"partyIdType": "MSISDN",
			"partyId":     req.Phone,
		},
		"payerMessage": note,
		"payeeNote":    note,
	}
	if err := p.request(ctx, item, http.MethodPost, path, ref, body, nil); err != nil {
		return nil, err
	}
	return &providers.ProviderPaymentResponse{
		ProviderReference: ref,
		Status:            domain.TxProcessing,
		Message:           "MTN request accepted",
		Raw: map[string]any{
			"mtn_reference_id": ref,
			"external_id":      req.Reference,
		},
	}, nil
}
func (p *Provider) QueryTransaction(ctx context.Context, ref, _ string) (*providers.ProviderTransactionStatus, error) {
	if ref == "" {
		return nil, errors.New("provider reference is required")
	}
	var last error
	for _, entry := range []struct {
		product
		path string
	}{
		{p.cfg.Collection, "/collection/v1_0/requesttopay/" + ref},
		{p.cfg.Disbursement, "/disbursement/v1_0/transfer/" + ref},
	} {
		var body map[string]any
		if !entry.valid() {
			continue
		}
		if err := p.request(ctx, entry.product, http.MethodGet, entry.path, "", nil, &body); err != nil {
			last = err
			continue
		}
		status := providers.Path(body, "status")
		return &providers.ProviderTransactionStatus{
			ProviderReference: ref,
			Status:            providers.PaymentStatus(status),
			Message:           providers.First(status, "MTN transaction status"),
		}, nil
	}
	return nil, providers.FirstError(last, errors.New("mtn credentials are incomplete"))
}
func (p *Provider) QueryBalance(ctx context.Context, _ string) (*providers.ProviderBalance, error) {
	var last error
	for _, entry := range []struct {
		product
		path string
	}{
		{p.cfg.Collection, "/collection/v1_0/account/balance"},
		{p.cfg.Disbursement, "/disbursement/v1_0/account/balance"},
	} {
		var body map[string]any
		if !entry.valid() {
			continue
		}
		if err := p.request(ctx, entry.product, http.MethodGet, entry.path, "", nil, &body); err != nil {
			last = err
			continue
		}
		currency := providers.First(providers.Path(body, "currency"), p.cfg.Currency)
		amount, err := providers.ParseAmountToMinor(providers.Path(body, "availableBalance"), currency)
		return &providers.ProviderBalance{Currency: currency, Available: amount, Ledger: amount}, err
	}
	return nil, providers.FirstError(last, errors.New("mtn credentials are incomplete"))
}
func (*Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*providers.ProviderWebhookEvent, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	currency := providers.First(providers.Path(body, "currency"), providers.Path(body, "data.currency"))
	amount, err := providers.OptionalAmount(providers.First(providers.Path(body, "amount"), providers.Path(body, "data.amount")), currency)
	if err != nil {
		return nil, err
	}
	return &providers.ProviderWebhookEvent{
		ProviderReference: providers.First(
			providers.Path(body, "referenceId"),
			providers.Path(body, "reference_id"),
			providers.Path(body, "provider_reference"),
			providers.Path(body, "transactionId"),
			providers.Path(body, "financialTransactionId"),
			headers["X-Reference-Id"],
		),
		Status:    providers.PaymentStatus(providers.Path(body, "status")),
		EventType: "mtn.payment.updated",
		ExternalReference: providers.First(
			providers.Path(body, "externalId"),
			providers.Path(body, "external_id"),
			providers.Path(body, "data.externalId"),
		),
		Amount:   amount,
		Currency: currency,
		Phone: providers.First(
			providers.Path(body, "payer.partyId"),
			providers.Path(body, "payee.partyId"),
			providers.Path(body, "data.payer.partyId"),
			providers.Path(body, "data.payee.partyId"),
		),
		Raw: body,
	}, nil
}
func (p *Provider) request(ctx context.Context, item product, method, path, ref string, in, out any) error {
	access, err := p.token(ctx, item)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"Authorization":             "Bearer " + access,
		"Ocp-Apim-Subscription-Key": item.key,
		"X-Target-Environment":      p.cfg.TargetEnvironment,
		"X-Reference-Id":            ref,
	}
	if ref != "" {
		headers["X-Callback-Url"] = p.cfg.CallbackURL
	}
	return providers.DoJSON(ctx, p.client, method, p.cfg.BaseURL+path, headers, in, out)
}
func (p *Provider) token(ctx context.Context, item product) (string, error) {
	if !item.valid() {
		return "", fmt.Errorf("mtn %s credentials are incomplete", item.name)
	}
	return p.tokens[item.name].Get(func() (string, time.Duration, error) {
		headers := map[string]string{
			"Authorization": "Basic " + base64.StdEncoding.EncodeToString(
				[]byte(item.user+":"+item.secret),
			),
			"Ocp-Apim-Subscription-Key": item.key,
		}
		var out struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		if err := providers.DoJSON(ctx, p.client, http.MethodPost, p.cfg.BaseURL+"/"+item.name+"/token/", headers, nil, &out); err != nil {
			return "", 0, err
		}
		if out.AccessToken == "" {
			return "", 0, errors.New("mtn token response missing access_token")
		}
		return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
	})
}
func (p product) valid() bool {
	return p.user != "" && p.secret != "" && p.key != ""
}
func parseConfig(raw providers.ProviderConfig) (Config, error) {
	cfg := Config{BaseURL: baseURL, TargetEnvironment: "sandbox", Currency: "UGX", Timeout: 30 * time.Second}
	cfg.BaseURL = strings.TrimRight(providers.First(providers.String(raw, "base_url"), cfg.BaseURL), "/")
	cfg.TargetEnvironment = providers.First(providers.String(raw, "target_environment"), cfg.TargetEnvironment)
	cfg.Currency, cfg.CallbackURL =
		strings.ToUpper(providers.First(providers.String(raw, "currency"), cfg.Currency)),
		providers.String(raw, "callback_url")
	cfg.Collection = product{
		"collection",
		providers.String(raw, "collection_subscription_key"),
		providers.String(raw, "collection_api_user"),
		providers.String(raw, "collection_api_key"),
	}
	cfg.Disbursement = product{
		"disbursement",
		providers.String(raw, "disbursement_subscription_key"),
		providers.String(raw, "disbursement_api_user"),
		providers.String(raw, "disbursement_api_key"),
	}
	if seconds := providers.Int(raw, "timeout_seconds"); seconds > 0 {
		cfg.Timeout = time.Duration(seconds) * time.Second
	}
	if !strings.HasPrefix(cfg.BaseURL, "https://") &&
		!(strings.HasPrefix(cfg.BaseURL, "http://") && providers.Bool(raw, "allow_insecure_http")) {
		return cfg, errors.New("mtn base_url must use https unless allow_insecure_http=true")
	}
	return cfg, nil
}
