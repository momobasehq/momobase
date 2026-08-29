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

const (
	defaultBaseURL              = "https://sandbox.momodeveloper.mtn.com"
	defaultProviderCallbackHost = "localhost"
)

// Config contains the settings recognized in an MTN Mobile Money provider
// account's configuration.
type Config struct {
	// Environment selects sandbox or production behavior.
	Environment string
	// BaseURL is the MTN MoMo API origin. It defaults to the sandbox origin and is
	// read from "base_url". Production accounts must provide it.
	BaseURL string
	// TargetEnvironment is sent in X-Target-Environment. It defaults to sandbox
	// and is read from "target_environment".
	TargetEnvironment string
	// APIUser is the OAuth API user read from "api_user".
	APIUser string
	// APIKey is the OAuth API key read from "api_key".
	APIKey string
	// ProviderCallbackHost is used when provisioning a sandbox API user. It is
	// read from "provider_callback_host", then derived from CallbackURL, and
	// finally defaults to localhost.
	ProviderCallbackHost string
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
	// WebhookSecret authenticates callbacks at Momobase's webhook boundary.
	WebhookSecret string
}

func parseConfig(raw providers.ProviderConfig) (Config, error) {
	environment := strings.ToLower(utils.First(providers.ConfigString(raw, "environment"), "sandbox"))
	baseURL := providers.ConfigString(raw, "base_url")
	targetEnvironment := providers.ConfigString(raw, "target_environment")
	if environment == "sandbox" {
		baseURL = utils.First(baseURL, defaultBaseURL)
		targetEnvironment = utils.First(targetEnvironment, "sandbox")
	}
	cfg := Config{
		Environment:                 environment,
		BaseURL:                     strings.TrimRight(baseURL, "/"),
		TargetEnvironment:           targetEnvironment,
		APIUser:                     providers.ConfigString(raw, "api_user"),
		APIKey:                      providers.ConfigString(raw, "api_key"),
		ProviderCallbackHost:        strings.ToLower(providers.ConfigString(raw, "provider_callback_host")),
		CollectionSubscriptionKey:   providers.ConfigString(raw, "collection_subscription_key"),
		DisbursementSubscriptionKey: providers.ConfigString(raw, "disbursement_subscription_key"),
		BalanceService:              strings.ToLower(providers.ConfigString(raw, "balance_service")),
		CallbackURL:                 providers.ConfigString(raw, "callback_url"),
		TransferType:                providers.ConfigString(raw, "transfer_type"),
		WebhookSecret:               providers.ConfigString(raw, "webhook_secret"),
	}
	if cfg.Environment != "sandbox" && cfg.Environment != "production" {
		return Config{}, errors.New("mtn: environment must be sandbox or production")
	}
	credentialsIncomplete := cfg.APIUser == "" || cfg.APIKey == ""
	credentialsPartiallySet := cfg.APIUser != "" || cfg.APIKey != ""
	if cfg.Environment == "production" && credentialsIncomplete {
		return Config{}, errors.New("mtn: api_user and api_key are required in production")
	}
	if cfg.Environment == "sandbox" && credentialsPartiallySet && credentialsIncomplete {
		return Config{}, errors.New("mtn: sandbox api_user and api_key must be provided together")
	}
	if cfg.Environment == "production" && cfg.BaseURL == "" {
		return Config{}, errors.New("mtn: base_url is required in production")
	}
	if cfg.Environment == "production" && cfg.TargetEnvironment == "" {
		return Config{}, errors.New("mtn: target_environment is required in production")
	}
	if cfg.WebhookSecret == "" {
		return Config{}, errors.New("mtn: webhook_secret is required")
	}
	if cfg.CollectionSubscriptionKey == "" && cfg.DisbursementSubscriptionKey == "" {
		return Config{}, errors.New("mtn: at least one product subscription key is required")
	}
	if cfg.ProviderCallbackHost == "" && cfg.CallbackURL != "" {
		callback, _ := url.Parse(cfg.CallbackURL)
		cfg.ProviderCallbackHost = strings.ToLower(callback.Hostname())
	}
	if cfg.ProviderCallbackHost == "" {
		cfg.ProviderCallbackHost = defaultProviderCallbackHost
	}
	if err := validateProviderCallbackHost(cfg.ProviderCallbackHost); err != nil {
		return Config{}, err
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
		"environment":                   cfg.Environment,
		"api_user":                      cfg.APIUser,
		"api_key":                       cfg.APIKey,
		"provider_callback_host":        cfg.ProviderCallbackHost,
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

func validateProviderCallbackHost(host string) error {
	if host == defaultProviderCallbackHost {
		return nil
	}
	parsed, err := url.Parse("https://" + host)
	isHostname := err == nil && parsed.Hostname() == host
	if !isHostname || net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		return errors.New("mtn: provider_callback_host must be a domain name")
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

func (c Config) provisioningKey() string {
	return utils.First(c.CollectionSubscriptionKey, c.DisbursementSubscriptionKey)
}
