package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// AdminService manages provider accounts, encrypted configuration, and runtime activation.
type AdminService struct {
	repos     *repository.UnitOfWork
	audit     *audit.Service
	encryptor *platform.Encryptor
	registry  providers.Registry
	runtime   *RuntimeManager
}

// CreateAccountInput contains the public settings for a provider account.
type CreateAccountInput struct {
	ProviderCode string
	Name         string
	Environment  string
	Country      string
	Currency     string
	Charges      *domain.ChargeSchedule
	Config       providers.ProviderConfig
}

// AccountSettings contains the routing location and future transaction charges.
type AccountSettings struct {
	Country  string
	Currency string
	Charges  domain.ChargeSchedule
}

// NewAdminService creates a provider administration service.
func NewAdminService(
	repos *repository.UnitOfWork,
	audit *audit.Service,
	enc *platform.Encryptor,
	registry providers.Registry,
	runtime *RuntimeManager,
) *AdminService {
	return &AdminService{repos: repos, audit: audit, encryptor: enc, registry: registry, runtime: runtime}
}

// RegisteredProviders returns the provider codes this build can create accounts
// for, in ascending order.
func (s *AdminService) RegisteredProviders() []string {
	return s.registry.List()
}

// CreateAccount validates and persists an inactive provider account with encrypted configuration.
func (s *AdminService) CreateAccount(
	ctx context.Context,
	actor *domain.AdminUser,
	input CreateAccountInput,
) (*domain.ProviderAccount, error) {
	if !s.registry.Has(input.ProviderCode) {
		return nil, fmt.Errorf("provider factory not registered: %s", input.ProviderCode)
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return nil, errors.New("provider name is required")
	}
	if input.Environment != "sandbox" && input.Environment != "production" {
		return nil, errors.New("environment must be sandbox or production")
	}
	country, err := utils.NormalizeTransactionCountry(input.Country)
	if err != nil {
		return nil, err
	}
	currency, err := utils.NormalizeCurrency(input.Currency)
	if err != nil {
		return nil, err
	}
	charges := domain.ChargeSchedule{}
	if input.Charges == nil {
		charges.Normalize()
	} else {
		charges = *input.Charges
	}
	if err = charges.Validate(); err != nil {
		return nil, err
	}
	cleanProviderConfig(input.Config)
	if err = validateProviderConfig(input.Config); err != nil {
		return nil, err
	}
	cipher, hash, err := s.encode(input.Config)
	if err != nil {
		return nil, err
	}
	account := &domain.ProviderAccount{
		BaseModel:           domain.BaseModel{ID: platform.NewID("pacc")},
		ProviderCode:        input.ProviderCode,
		Name:                input.Name,
		Environment:         input.Environment,
		Country:             country,
		Currency:            currency,
		Charges:             charges,
		ConfigVersion:       1,
		EncryptedConfigJSON: cipher,
		ConfigHash:          hash,
	}
	if err = s.repos.ProviderAccounts.Create(ctx, account); err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(
		ctx,
		actor.ActorID(),
		"admin",
		"provider_account.created",
		"provider_account",
		account.ID,
		map[string]any{
			"provider_code": input.ProviderCode,
			"country":       country,
			"currency":      currency,
			"charges":       charges,
		},
		"",
		"",
	)
	return account, nil
}

// UpdateSettings replaces a provider account's location and charges, reloading an
// active runtime so its country metadata changes at the same time as the row.
func (s *AdminService) UpdateSettings(
	ctx context.Context,
	actor *domain.AdminUser,
	id string,
	settings AccountSettings,
) error {
	country, err := utils.NormalizeTransactionCountry(settings.Country)
	if err != nil {
		return err
	}
	currency, err := utils.NormalizeCurrency(settings.Currency)
	if err != nil {
		return err
	}
	if err := settings.Charges.Validate(); err != nil {
		return err
	}
	account, err := s.repos.ProviderAccounts.ByID(ctx, id)
	if err != nil {
		return err
	}
	plain, err := s.runtime.plain(account)
	if err != nil {
		return err
	}
	if err = validateProviderConfig(plain); err != nil {
		return err
	}
	updates := settingsUpdates(country, currency, settings.Charges)
	if err = s.repos.ProviderAccounts.Update(ctx, id, updates); err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			if rollback := s.repos.ProviderAccounts.Restore(ctx, id, settingsUpdates(
				account.Country,
				account.Currency,
				account.Charges,
			)); rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(
		ctx,
		actor.ActorID(),
		"admin",
		"provider_settings.updated",
		"provider_account",
		id,
		map[string]any{"country": country, "currency": currency, "charges": settings.Charges},
		"",
		"",
	)
	return nil
}

func settingsUpdates(country, currency string, charges domain.ChargeSchedule) map[string]any {
	return map[string]any{
		"country":                   country,
		"currency":                  currency,
		"collection_charge_type":    charges.Collection.Type,
		"collection_charge_value":   charges.Collection.Value,
		"disbursement_charge_type":  charges.Disbursement.Type,
		"disbursement_charge_value": charges.Disbursement.Value,
	}
}

// UpdateConfig validates and encrypts replacement provider configuration and reloads an active runtime.
func (s *AdminService) UpdateConfig(
	ctx context.Context,
	actor *domain.AdminUser,
	id string,
	config providers.ProviderConfig,
) error {
	account, err := s.repos.ProviderAccounts.ByID(ctx, id)
	if err != nil {
		return err
	}
	cleanProviderConfig(config)
	if err := validateProviderConfig(config); err != nil {
		return err
	}
	cipher, hash, err := s.encode(config)
	if err != nil {
		return err
	}
	if err = s.repos.ProviderAccounts.SetConfig(ctx, id, cipher, hash); err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			restore := map[string]any{
				"encrypted_config_json": account.EncryptedConfigJSON,
				"config_hash":           account.ConfigHash,
				"config_version":        account.ConfigVersion,
			}
			if rollback := s.repos.ProviderAccounts.Restore(ctx, id, restore); rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider_config.updated", "provider_account", id, nil, "", "")
	return nil
}

// Activate marks a configured provider account active and loads its runtime adapter.
func (s *AdminService) Activate(ctx context.Context, actor *domain.AdminUser, id string) error {
	if _, err := s.repos.ProviderAccounts.ByID(ctx, id); err != nil {
		return err
	}
	if err := s.repos.ProviderAccounts.SetActive(ctx, id, true); err != nil {
		return err
	}
	if err := s.runtime.Reload(ctx, id); err != nil {
		// The row is committed before the adapter is built, so a reload that fails has
		// to put it back: an account marked active with no runtime would route to
		// nothing.
		_ = s.repos.ProviderAccounts.Restore(ctx, id, map[string]any{"active": false})
		return err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider.activated", "provider_account", id, nil, "", "")
	return nil
}

// Deactivate marks a provider account inactive and removes its runtime adapter.
func (s *AdminService) Deactivate(ctx context.Context, actor *domain.AdminUser, id string) error {
	if err := s.repos.ProviderAccounts.SetActive(ctx, id, false); err != nil {
		return err
	}
	s.runtime.Disable(id)
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider.deactivated", "provider_account", id, nil, "", "")
	return nil
}

func (s *AdminService) encode(config providers.ProviderConfig) (string, string, error) {
	plain, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	cipher, err := s.encryptor.Encrypt(plain)
	return cipher, platform.SHA256Hex(string(plain)), err
}

func cleanProviderConfig(config providers.ProviderConfig) {
	delete(config, "country")
	delete(config, "supports_global")
}

func validateProviderConfig(config providers.ProviderConfig) error {
	if len(config) == 0 {
		return errors.New("provider config is required")
	}
	if providers.ConfigString(config, "webhook_secret") == "" {
		return errors.New("webhook_secret is required")
	}
	return nil
}
