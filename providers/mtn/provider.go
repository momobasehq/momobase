package mtn

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/providers"
)

const (
	collectionPath   = "/collection/v1_0/requesttopay"
	disbursementPath = "/disbursement/v1_0/transfer"
	tokenLeeway      = 30 * time.Second
)

type cachedToken struct {
	value     string
	expiresAt time.Time
}

// Provider connects Momobase to the MTN Mobile Money collection and
// disbursement products.
type Provider struct {
	log    *slog.Logger
	client *http.Client

	mu     sync.RWMutex
	cfg    Config
	tokens map[string]cachedToken
}

var (
	_ providers.PaymentProvider  = (*Provider)(nil)
	_ providers.RequestValidator = (*Provider)(nil)
)

// New returns an MTN Mobile Money provider ready to be initialized.
func New(log *slog.Logger) providers.PaymentProvider {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Provider{
		log:    log,
		client: &http.Client{Timeout: 30 * time.Second},
		tokens: map[string]cachedToken{},
	}
}

// Capabilities returns the services enabled by the configured product
// subscription keys.
func (p *Provider) Capabilities() []providers.Capability {
	cfg := p.config()
	capabilities := []providers.Capability{}
	if cfg.CollectionSubscriptionKey != "" {
		capabilities = append(capabilities, providers.Capability{ServiceType: domain.ServiceCollection})
	}
	if cfg.DisbursementSubscriptionKey != "" {
		capabilities = append(capabilities, providers.Capability{ServiceType: domain.ServiceDisbursement})
	}
	return capabilities
}

// Init validates and applies the provider configuration, provisioning sandbox
// credentials when needed and clearing any cached product access tokens.
func (p *Provider) Init(ctx context.Context, raw providers.ProviderConfig) error {
	cfg, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if cfg.Environment == "sandbox" && cfg.APIUser == "" {
		cfg, err = p.provisionSandbox(ctx, cfg)
		if err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.cfg = cfg
	p.tokens = map[string]cachedToken{}
	p.mu.Unlock()
	p.log.Info(
		"mtn momo provider initialized",
		slog.String("environment", cfg.Environment),
		slog.String("target_environment", cfg.TargetEnvironment),
		slog.Int("services", len(p.Capabilities())),
	)
	return nil
}

func (p *Provider) provisionSandbox(ctx context.Context, cfg Config) (Config, error) {
	apiUser := uuid.NewString()
	headers := map[string]string{
		"Ocp-Apim-Subscription-Key": cfg.provisioningKey(),
		"X-Reference-Id":            apiUser,
	}
	body := sandboxUserRequest{ProviderCallbackHost: cfg.ProviderCallbackHost}
	endpoint := cfg.BaseURL + "/v1_0/apiuser"
	if err := providers.DoJSON(ctx, p.client, http.MethodPost, endpoint, headers, body, nil); err != nil {
		return Config{}, fmt.Errorf("mtn: provision sandbox api user: %w", err)
	}
	var out sandboxAPIKeyResponse
	endpoint += "/" + url.PathEscape(apiUser) + "/apikey"
	apiKeyHeaders := map[string]string{
		"Ocp-Apim-Subscription-Key": cfg.provisioningKey(),
	}
	if err := providers.DoJSON(ctx, p.client, http.MethodPost, endpoint, apiKeyHeaders, nil, &out); err != nil {
		return Config{}, fmt.Errorf("mtn: provision sandbox api key: %w", err)
	}
	apiKey := strings.TrimSpace(out.APIKey)
	if apiKey == "" {
		return Config{}, errors.New("mtn: sandbox api key response is incomplete")
	}
	cfg.APIUser = apiUser
	cfg.APIKey = apiKey
	return cfg, nil
}

// HealthCheck verifies that every enabled product can issue an OAuth access
// token with the configured credentials.
func (p *Provider) HealthCheck(ctx context.Context) error {
	capabilities := p.Capabilities()
	if len(capabilities) == 0 {
		return errors.New("mtn: provider is not initialized")
	}
	for _, capability := range capabilities {
		if _, err := p.accessToken(ctx, capability.ServiceType); err != nil {
			return fmt.Errorf("mtn: health check %s: %w", capability.ServiceType, err)
		}
	}
	return nil
}

// ValidateRequest validates an MTN MoMo MSISDN and normalizes its formatting and
// scheme before Momobase persists it.
func (p *Provider) ValidateRequest(_ context.Context, req *providers.PaymentRequest) error {
	if req == nil {
		return errors.New("mtn: payment request is required")
	}
	account, err := normalizeMSISDN(req.Account)
	if err != nil {
		return fmt.Errorf("mtn: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(req.Scheme))
	switch scheme {
	case "", "mtn", "mtn_momo", "momo":
		req.Scheme = "mtn_momo"
	default:
		return fmt.Errorf("mtn: unsupported account scheme %q", req.Scheme)
	}
	req.Account = account
	return nil
}

// Collect requests a payment from an MTN MoMo customer.
func (p *Provider) Collect(
	ctx context.Context,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	return p.pay(ctx, domain.ServiceCollection, req)
}

// Disburse transfers money to an MTN MoMo customer.
func (p *Provider) Disburse(
	ctx context.Context,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	return p.pay(ctx, domain.ServiceDisbursement, req)
}

func (p *Provider) pay(
	ctx context.Context,
	service string,
	req providers.PaymentRequest,
) (*providers.ProviderPaymentResponse, error) {
	cfg := p.config()
	if cfg.subscriptionKey(service) == "" {
		return nil, fmt.Errorf("mtn: %s is not enabled for this account", service)
	}
	token, err := p.accessToken(ctx, service)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	party := party{PartyIDType: "MSISDN", PartyID: req.Account}
	body := paymentRequest{
		Amount:       providers.FormatAmountMinor(req.Amount, req.Currency),
		Currency:     strings.ToUpper(req.Currency),
		ExternalID:   id,
		PayerMessage: narrative(req),
		PayeeNote:    narrative(req),
		TransferType: cfg.TransferType,
	}
	path := collectionPath
	if service == domain.ServiceCollection {
		body.Payer = &party
	} else {
		path = disbursementPath
		body.Payee = &party
	}
	headers := p.apiHeaders(cfg, service, token)
	headers["X-Reference-Id"] = id
	if cfg.CallbackURL != "" {
		headers["X-Callback-Url"] = cfg.CallbackURL
	}
	if err := providers.DoJSON(ctx, p.client, http.MethodPost, cfg.BaseURL+path, headers, body, nil); err != nil {
		return nil, fmt.Errorf("mtn: submit %s: %w", service, err)
	}
	reference := makeReference(service, id)
	return &providers.ProviderPaymentResponse{
		ProviderReference: reference,
		Status:            domain.TxProcessing,
		Message:           "accepted by MTN MoMo",
		Raw: map[string]any{
			"reference": reference,
			"service":   service,
			"status":    "PENDING",
		},
	}, nil
}

// QueryTransaction retrieves an MTN MoMo transaction using the service encoded
// in its provider reference.
func (p *Provider) QueryTransaction(
	ctx context.Context,
	providerReference string,
	_ string,
) (*providers.ProviderTransactionStatus, error) {
	service, id, err := parseReference(providerReference)
	if err != nil {
		return nil, err
	}
	cfg := p.config()
	if cfg.subscriptionKey(service) == "" {
		return nil, fmt.Errorf("mtn: %s is not enabled for this account", service)
	}
	token, err := p.accessToken(ctx, service)
	if err != nil {
		return nil, err
	}
	path := collectionPath
	if service == domain.ServiceDisbursement {
		path = disbursementPath
	}
	var out transactionStatus
	endpoint := cfg.BaseURL + path + "/" + url.PathEscape(id)
	if err := providers.DoJSON(ctx, p.client, http.MethodGet, endpoint, p.apiHeaders(cfg, service, token), nil, &out); err != nil {
		return nil, fmt.Errorf("mtn: query %s: %w", service, err)
	}
	if strings.TrimSpace(out.Status) == "" {
		return nil, fmt.Errorf("mtn: query %s response has no status", service)
	}
	return &providers.ProviderTransactionStatus{
		ProviderReference: providerReference,
		Status:            providers.PaymentStatus(out.Status),
		Message:           out.message(),
	}, nil
}

// QueryBalance retrieves the balance of the product selected by balance_service.
func (p *Provider) QueryBalance(ctx context.Context, _ string) (*providers.ProviderBalance, error) {
	cfg := p.config()
	service := cfg.BalanceService
	if cfg.subscriptionKey(service) == "" {
		return nil, errors.New("mtn: provider is not initialized")
	}
	token, err := p.accessToken(ctx, service)
	if err != nil {
		return nil, err
	}
	var out balanceResponse
	endpoint := fmt.Sprintf("%s/%s/v1_0/account/balance", cfg.BaseURL, service)
	if err := providers.DoJSON(ctx, p.client, http.MethodGet, endpoint, p.apiHeaders(cfg, service, token), nil, &out); err != nil {
		return nil, fmt.Errorf("mtn: query %s balance: %w", service, err)
	}
	if strings.TrimSpace(out.AvailableBalance) == "" || strings.TrimSpace(out.Currency) == "" {
		return nil, fmt.Errorf("mtn: %s balance response is incomplete", service)
	}
	available, err := providers.ParseAmountToMinor(out.AvailableBalance, out.Currency)
	if err != nil {
		return nil, fmt.Errorf("mtn: parse available balance: %w", err)
	}
	return &providers.ProviderBalance{
		Currency:  strings.ToUpper(out.Currency),
		Available: available,
		Ledger:    available,
	}, nil
}

func (p *Provider) accessToken(ctx context.Context, service string) (string, error) {
	now := time.Now()
	p.mu.RLock()
	cfg := p.cfg
	cached := p.tokens[service]
	p.mu.RUnlock()
	if cached.value != "" && now.Add(tokenLeeway).Before(cached.expiresAt) {
		return cached.value, nil
	}
	if cfg.subscriptionKey(service) == "" {
		return "", fmt.Errorf("mtn: %s is not enabled for this account", service)
	}
	issued, err := p.requestToken(ctx, cfg, service)
	if err != nil {
		return "", err
	}
	p.mu.Lock()
	if current := p.tokens[service]; current.expiresAt.After(issued.expiresAt) {
		issued = current
	} else {
		p.tokens[service] = issued
	}
	p.mu.Unlock()
	return issued.value, nil
}

func (p *Provider) requestToken(ctx context.Context, cfg Config, service string) (cachedToken, error) {
	headers := map[string]string{
		"Authorization":             "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.APIUser+":"+cfg.APIKey)),
		"Ocp-Apim-Subscription-Key": cfg.subscriptionKey(service),
	}
	var out tokenResponse
	endpoint := fmt.Sprintf("%s/%s/token/", cfg.BaseURL, service)
	if err := providers.DoJSON(ctx, p.client, http.MethodPost, endpoint, headers, nil, &out); err != nil {
		return cachedToken{}, fmt.Errorf("mtn: request %s access token: %w", service, err)
	}
	if out.AccessToken == "" || out.ExpiresIn <= 0 {
		return cachedToken{}, fmt.Errorf("mtn: %s access token response is incomplete", service)
	}
	return cachedToken{
		value:     out.AccessToken,
		expiresAt: time.Now().Add(time.Duration(out.ExpiresIn) * time.Second),
	}, nil
}

func (p *Provider) apiHeaders(cfg Config, service, token string) map[string]string {
	return map[string]string{
		"Authorization":             "Bearer " + token,
		"Ocp-Apim-Subscription-Key": cfg.subscriptionKey(service),
		"X-Target-Environment":      cfg.TargetEnvironment,
	}
}

func (p *Provider) config() Config {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cfg
}

func makeReference(service, id string) string { return service + ":" + id }

func parseReference(reference string) (string, string, error) {
	service, id, ok := strings.Cut(reference, ":")
	if !ok || service != domain.ServiceCollection && service != domain.ServiceDisbursement {
		return "", "", fmt.Errorf("mtn: invalid provider reference %q", reference)
	}
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 4 {
		return "", "", fmt.Errorf("mtn: invalid provider reference %q", reference)
	}
	return service, parsed.String(), nil
}

func narrative(req providers.PaymentRequest) string {
	value := strings.TrimSpace(req.Description)
	if value == "" {
		value = strings.TrimSpace(req.Reference)
	}
	runes := []rune(value)
	if len(runes) > 160 {
		return string(runes[:160])
	}
	return value
}
