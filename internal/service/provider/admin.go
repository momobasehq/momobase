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
	code string,
	name string,
	environment string,
	countries []string,
	config map[string]any,
) (*domain.ProviderAccount, error) {
	if !s.registry.Has(code) {
		return nil, fmt.Errorf("provider factory not registered: %s", code)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("provider name is required")
	}
	if environment != "sandbox" && environment != "production" {
		return nil, errors.New("environment must be sandbox or production")
	}
	countries, err := utils.NormalizeProviderCountries(countries)
	if err != nil {
		return nil, err
	}
	cleanProviderConfig(config)
	if err = validateProviderConfig(config); err != nil {
		return nil, err
	}
	cipher, hash, err := s.encode(config)
	if err != nil {
		return nil, err
	}
	account := &domain.ProviderAccount{
		BaseModel: domain.BaseModel{ID: platform.NewID("pacc")}, ProviderCode: code, Name: name,
		Environment: environment, Countries: countries, ConfigVersion: 1,
		EncryptedConfigJSON: cipher, ConfigHash: hash,
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
		map[string]any{"provider_code": code, "countries": countries},
		"",
		"",
	)
	return account, nil
}

// UpdateCountries replaces a provider account's supported countries and reloads an active runtime.
func (s *AdminService) UpdateCountries(ctx context.Context, actor *domain.AdminUser, id string, countries []string) error {
	countries, err := utils.NormalizeProviderCountries(countries)
	if err != nil {
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
	if err = s.updateCountries(ctx, id, countries); err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			if rollback := s.updateCountries(ctx, id, account.Countries); rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(
		ctx,
		actor.ActorID(),
		"admin",
		"provider_countries.updated",
		"provider_account",
		id,
		map[string]any{"countries": countries},
		"",
		"",
	)
	return nil
}

func (s *AdminService) updateCountries(ctx context.Context, id string, countries []string) error {
	raw, err := json.Marshal(countries)
	if err != nil {
		return err
	}
	return s.repos.ProviderAccounts.SetCountries(ctx, id, string(raw))
}

// UpdateConfig validates and encrypts replacement provider configuration and reloads an active runtime.
func (s *AdminService) UpdateConfig(ctx context.Context, actor *domain.AdminUser, id string, config map[string]any) error {
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

func (s *AdminService) encode(config map[string]any) (string, string, error) {
	plain, err := json.Marshal(config)
	if err != nil {
		return "", "", err
	}
	cipher, err := s.encryptor.Encrypt(plain)
	return cipher, platform.SHA256Hex(string(plain)), err
}

func cleanProviderConfig(config map[string]any) {
	delete(config, "country")
	delete(config, "supports_global")
}

func validateProviderConfig(config map[string]any) error {
	if len(config) == 0 {
		return errors.New("provider config is required")
	}
	if utils.String(config, "webhook_secret") == "" {
		return errors.New("webhook_secret is required")
	}
	return nil
}
