package airtel

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"momobase/internal/domain"
	"momobase/internal/providers"
)

type Config struct {
	BaseURL, ClientID, ClientSecret, Country, Currency        string
	CollectionPath, DisbursementPath, StatusPath, BalancePath string
	CollectionEnabled, DisbursementEnabled                    bool
	Timeout                                                   time.Duration
}
type Provider struct {
	client *http.Client
	cfg    Config
	token  providers.TokenCache
}

func New(*slog.Logger) providers.PaymentProvider {
	return &Provider{client: &http.Client{Timeout: 30 * time.Second}}
}
func (p *Provider) Capabilities() []providers.Capability {
	all := []providers.Capability{{ServiceType: domain.ServiceCollection, PaymentMethod: domain.PaymentMethodMomo}, {ServiceType: domain.ServiceDisbursement, PaymentMethod: domain.PaymentMethodMomo}}
	if !p.credentials() {
		return nil
	}
	out := make([]providers.Capability, 0, 2)
	if p.cfg.CollectionEnabled {
		out = append(out, all[0])
	}
	if p.cfg.DisbursementEnabled {
		out = append(out, all[1])
	}
	return out
}
func (p *Provider) Init(_ context.Context, raw providers.ProviderConfig) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	p.cfg, p.client, p.token = cfg, &http.Client{Timeout: cfg.Timeout}, providers.TokenCache{}
	return nil
}
func (p *Provider) HealthCheck(ctx context.Context) error { _, err := p.accessToken(ctx); return err }
func (p *Provider) Collect(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.payment(ctx, p.cfg.CollectionEnabled, p.cfg.CollectionPath, "subscriber", req)
}
func (p *Provider) Disburse(ctx context.Context, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	return p.payment(ctx, p.cfg.DisbursementEnabled, p.cfg.DisbursementPath, "payee", req)
}
func (p *Provider) payment(ctx context.Context, enabled bool, path, party string, req providers.PaymentRequest) (*providers.ProviderPaymentResponse, error) {
	if !p.credentials() || !enabled {
		return nil, errors.New("airtel payment operation is not configured")
	}
	country, currency, ref := upper(req.Country, p.cfg.Country), upper(req.Currency, p.cfg.Currency), providers.RandomRef("airtel-")
	person := map[string]string{"country": country, "currency": currency, "msisdn": req.Phone}
	tx := map[string]string{"amount": providers.FormatAmountMinor(req.Amount, currency), "country": country, "currency": currency, "id": ref, "message": req.Description}
	raw, err := p.call(ctx, http.MethodPost, path, map[string]any{"reference": req.Reference, party: person, "transaction": tx})
	if err != nil {
		return nil, err
	}
	return &providers.ProviderPaymentResponse{ProviderReference: providers.First(providers.Path(raw, "transaction.id"), providers.Path(raw, "data.transaction.id"), ref), Status: domain.TxProcessing, Message: "Airtel request accepted", Raw: raw}, nil
}
func (p *Provider) QueryTransaction(ctx context.Context, ref string) (*providers.ProviderTransactionStatus, error) {
	if ref == "" {
		return nil, errors.New("provider reference is required")
	}
	raw, err := p.call(ctx, http.MethodGet, strings.ReplaceAll(p.cfg.StatusPath, "{id}", ref), nil)
	if err != nil {
		return nil, err
	}
	status := providers.First(providers.Path(raw, "transaction.status"), providers.Path(raw, "data.transaction.status"), providers.Path(raw, "status.code"), providers.Path(raw, "status"))
	return &providers.ProviderTransactionStatus{ProviderReference: ref, Status: providers.PaymentStatus(status), Message: providers.First(status, "Airtel transaction status")}, nil
}
func (p *Provider) QueryBalance(ctx context.Context) (*providers.ProviderBalance, error) {
	raw, err := p.call(ctx, http.MethodGet, p.cfg.BalancePath, nil)
	if err != nil {
		return nil, err
	}
	currency := providers.First(providers.Path(raw, "data.currency"), providers.Path(raw, "currency"), p.cfg.Currency)
	value, err := providers.ParseAmountToMinor(providers.First(providers.Path(raw, "data.balance"), providers.Path(raw, "balance"), providers.Path(raw, "available_balance")), currency)
	if err != nil {
		return nil, err
	}
	return &providers.ProviderBalance{Currency: currency, Available: value, Ledger: value}, nil
}
func (*Provider) VerifyWebhook(_ context.Context, payload []byte, headers map[string]string) (*providers.ProviderWebhookEvent, error) {
	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, err
	}
	currency := providers.First(providers.Path(body, "transaction.currency"), providers.Path(body, "data.transaction.currency"), providers.Path(body, "data.currency"), providers.Path(body, "currency"))
	amount, err := providers.OptionalAmount(providers.First(providers.Path(body, "transaction.amount"), providers.Path(body, "data.transaction.amount"), providers.Path(body, "data.amount"), providers.Path(body, "amount")), currency)
	if err != nil {
		return nil, err
	}
	status := providers.First(providers.Path(body, "transaction.status"), providers.Path(body, "data.transaction.status"), providers.Path(body, "status"))
	return &providers.ProviderWebhookEvent{ProviderReference: providers.First(providers.Path(body, "transaction.id"), providers.Path(body, "data.transaction.id"), providers.Path(body, "transaction_id"), providers.Path(body, "reference"), headers["X-Transaction-Id"]), Status: providers.PaymentStatus(status), EventType: "airtel.payment.updated", ExternalReference: providers.First(providers.Path(body, "transaction.reference"), providers.Path(body, "data.transaction.reference"), providers.Path(body, "external_id"), providers.Path(body, "externalId")), Amount: amount, Currency: currency, Phone: providers.First(providers.Path(body, "transaction.msisdn"), providers.Path(body, "data.transaction.msisdn"), providers.Path(body, "data.msisdn"), providers.Path(body, "msisdn")), Raw: body}, nil
}
func (p *Provider) credentials() bool { return p.cfg.ClientID != "" && p.cfg.ClientSecret != "" }
func (p *Provider) accessToken(ctx context.Context) (string, error) {
	return p.token.Get(func() (string, time.Duration, error) {
		if !p.credentials() {
			return "", 0, errors.New("airtel credentials are incomplete")
		}
		var out struct {
			AccessToken string `json:"access_token"`
			ExpiresIn   int    `json:"expires_in"`
		}
		in := map[string]string{"client_id": p.cfg.ClientID, "client_secret": p.cfg.ClientSecret, "grant_type": "client_credentials"}
		if err := providers.DoJSON(ctx, p.client, http.MethodPost, p.cfg.BaseURL+"/auth/oauth2/token", nil, in, &out); err != nil {
			return "", 0, err
		}
		if out.AccessToken == "" {
			return "", 0, errors.New("airtel token response missing access_token")
		}
		return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
	})
}
func (p *Provider) call(ctx context.Context, method, path string, in any) (map[string]any, error) {
	access, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	headers := map[string]string{"Authorization": "Bearer " + access, "X-Country": p.cfg.Country, "X-Currency": p.cfg.Currency}
	err = providers.DoJSON(ctx, p.client, method, p.cfg.BaseURL+path, headers, in, &out)
	return out, err
}
func parseConfig(raw providers.ProviderConfig) (Config, error) {
	cfg := Config{BaseURL: "https://openapiuat.airtel.africa", Country: "UG", Currency: "UGX", Timeout: 30 * time.Second, CollectionEnabled: true, DisbursementEnabled: true, CollectionPath: "/merchant/v1/payments/", DisbursementPath: "/standard/v1/disbursements/", StatusPath: "/standard/v1/payments/{id}", BalancePath: "/standard/v1/users/balance"}
	cfg.BaseURL = strings.TrimRight(providers.First(providers.String(raw, "base_url"), cfg.BaseURL), "/")
	cfg.ClientID, cfg.ClientSecret = providers.String(raw, "client_id"), providers.String(raw, "client_secret")
	cfg.Country, cfg.Currency = upper(providers.String(raw, "country"), cfg.Country), upper(providers.String(raw, "currency"), cfg.Currency)
	cfg.CollectionPath = providers.Slash(providers.First(providers.String(raw, "collection_path"), cfg.CollectionPath))
	cfg.DisbursementPath = providers.Slash(providers.First(providers.String(raw, "disbursement_path"), cfg.DisbursementPath))
	cfg.StatusPath = providers.Slash(providers.First(providers.String(raw, "status_path_template"), cfg.StatusPath))
	cfg.BalancePath = providers.Slash(providers.First(providers.String(raw, "balance_path"), cfg.BalancePath))
	if n := providers.Int(raw, "timeout_seconds"); n > 0 {
		cfg.Timeout = time.Duration(n) * time.Second
	}
	if _, ok := raw["collection_enabled"]; ok {
		cfg.CollectionEnabled = providers.Bool(raw, "collection_enabled")
	}
	if _, ok := raw["disbursement_enabled"]; ok {
		cfg.DisbursementEnabled = providers.Bool(raw, "disbursement_enabled")
	}
	if !strings.HasPrefix(cfg.BaseURL, "https://") && !(strings.HasPrefix(cfg.BaseURL, "http://") && providers.Bool(raw, "allow_insecure_http")) {
		return cfg, errors.New("airtel base_url must use https unless allow_insecure_http=true")
	}
	return cfg, nil
}
func upper(a, fallback string) string { return strings.ToUpper(providers.First(a, fallback)) }
