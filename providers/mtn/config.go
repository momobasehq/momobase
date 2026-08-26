package mtn

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"unicode"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

const defaultBaseURL = "https://sandbox.momodeveloper.mtn.com"

// Config contains the settings recognized in an MTN Mobile Money provider
// account's configuration.
type Config struct {
	// BaseURL is the MTN MoMo API origin. It defaults to the sandbox origin and is
	// read from "base_url".
	BaseURL string
	// TargetEnvironment is sent in X-Target-Environment. It defaults to sandbox
	// and is read from "target_environment".
	TargetEnvironment string
	// APIUser is the OAuth API user and is read from "api_user".
	APIUser string
	// APIKey is the OAuth API key and is read from "api_key".
	APIKey string
	// CollectionSubscriptionKey enables collections and is read from
	// "collection_subscription_key".
	CollectionSubscriptionKey string
	// DisbursementSubscriptionKey enables disbursements and is read from
	// "disbursement_subscription_key".
	DisbursementSubscriptionKey string
	// BalanceService selects which product account QueryBalance reads when both
	// services are enabled. It is read from "balance_service".
	BalanceService string
	// CallbackURL is sent in X-Callback-Url when nonblank and is read from
	// "callback_url".
	CallbackURL string
	// TransferType enables MTN's aggregator capability when nonblank and is read
	// from "transfer_type".
	TransferType string
	// WebhookSecret authenticates callbacks at Momobase's webhook boundary. It is
	// required and is read from "webhook_secret".
	WebhookSecret string
}

func parseConfig(raw providers.ProviderConfig) (Config, error) {
	cfg := Config{
		BaseURL:                     strings.TrimRight(utils.First(utils.String(raw, "base_url"), defaultBaseURL), "/"),
		TargetEnvironment:           utils.First(utils.String(raw, "target_environment"), "sandbox"),
		APIUser:                     utils.String(raw, "api_user"),
		APIKey:                      utils.String(raw, "api_key"),
		CollectionSubscriptionKey:   utils.String(raw, "collection_subscription_key"),
		DisbursementSubscriptionKey: utils.String(raw, "disbursement_subscription_key"),
		BalanceService:              strings.ToLower(utils.String(raw, "balance_service")),
		CallbackURL:                 utils.String(raw, "callback_url"),
		TransferType:                utils.String(raw, "transfer_type"),
		WebhookSecret:               utils.String(raw, "webhook_secret"),
	}
	if cfg.APIUser == "" || cfg.APIKey == "" {
		return Config{}, errors.New("mtn: api_user and api_key are required")
	}
	if cfg.WebhookSecret == "" {
		return Config{}, errors.New("mtn: webhook_secret is required")
	}
	if cfg.CollectionSubscriptionKey == "" && cfg.DisbursementSubscriptionKey == "" {
		return Config{}, errors.New("mtn: at least one product subscription key is required")
	}
	if err := validateConfigValues(cfg); err != nil {
		return Config{}, err
	}
	if cfg.BalanceService == "" {
		cfg.BalanceService = domain.ServiceCollection
		if cfg.CollectionSubscriptionKey == "" {
			cfg.BalanceService = domain.ServiceDisbursement
		}
	}
	if cfg.subscriptionKey(cfg.BalanceService) == "" {
		return Config{}, fmt.Errorf("mtn: balance_service %q is not enabled", cfg.BalanceService)
	}
	return cfg, nil
}

func validateConfigValues(cfg Config) error {
	for name, value := range map[string]string{
		"api_user":                      cfg.APIUser,
		"api_key":                       cfg.APIKey,
		"collection_subscription_key":   cfg.CollectionSubscriptionKey,
		"disbursement_subscription_key": cfg.DisbursementSubscriptionKey,
		"target_environment":            cfg.TargetEnvironment,
		"transfer_type":                 cfg.TransferType,
		"webhook_secret":                cfg.WebhookSecret,
	} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return fmt.Errorf("mtn: %s contains invalid control characters", name)
		}
	}
	if err := validateURL("base_url", cfg.BaseURL); err != nil {
		return err
	}
	if cfg.CallbackURL != "" {
		if err := validateURL("callback_url", cfg.CallbackURL); err != nil {
			return err
		}
	}
	return nil
}

func validateURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return fmt.Errorf("mtn: %s must be an absolute HTTP URL", name)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopback(parsed.Hostname()) {
			return nil
		}
	}
	return fmt.Errorf("mtn: %s must use HTTPS", name)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c Config) subscriptionKey(service string) string {
	switch service {
	case domain.ServiceCollection:
		return c.CollectionSubscriptionKey
	case domain.ServiceDisbursement:
		return c.DisbursementSubscriptionKey
	default:
		return ""
	}
}
