package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/audit"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/store"
	"github.com/momobasehq/momobase/internal/utils"
	"github.com/momobasehq/momobase/providers"
)

// ProviderAdminService manages provider accounts, encrypted configuration, and runtime activation.
type ProviderAdminService struct {
	db        *gorm.DB
	audit     *audit.Service
	encryptor *platform.Encryptor
	registry  providers.Registry
	runtime   *ProviderRuntimeManager
}

// NewProviderAdminService creates a provider administration service.
func NewProviderAdminService(
	db *gorm.DB,
	audit *audit.Service,
	enc *platform.Encryptor,
	registry providers.Registry,
	runtime *ProviderRuntimeManager,
) *ProviderAdminService {
	return &ProviderAdminService{db: db, audit: audit, encryptor: enc, registry: registry, runtime: runtime}
}

// RegisteredProviders returns the provider codes this build can create accounts
// for, in ascending order.
func (s *ProviderAdminService) RegisteredProviders() []string {
	return s.registry.List()
}

// CreateAccount validates and persists an inactive provider account with encrypted configuration.
func (s *ProviderAdminService) CreateAccount(
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
	if err = s.db.WithContext(ctx).Create(account).Error; err != nil {
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
func (s *ProviderAdminService) UpdateCountries(ctx context.Context, actor *domain.AdminUser, id string, countries []string) error {
	countries, err := utils.NormalizeProviderCountries(countries)
	if err != nil {
		return err
	}
	var account domain.ProviderAccount
	db := s.db.WithContext(ctx)
	if err = db.First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	plain, err := s.runtime.plain(&account)
	if err != nil {
		return err
	}
	if err = validateProviderConfig(plain); err != nil {
		return err
	}
	if err = updateProviderCountries(db, id, countries); err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			if rollback := updateProviderCountries(db, id, account.Countries); rollback != nil {
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

func updateProviderCountries(db *gorm.DB, id string, countries []string) error {
	raw, err := json.Marshal(countries)
	if err != nil {
		return err
	}
	return store.Affected(db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("countries", string(raw)))
}

// UpdateConfig validates and encrypts replacement provider configuration and reloads an active runtime.
func (s *ProviderAdminService) UpdateConfig(ctx context.Context, actor *domain.AdminUser, id string, config map[string]any) error {
	var account domain.ProviderAccount
	db := s.db.WithContext(ctx)
	if err := db.First(&account, "id = ?", id).Error; err != nil {
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
	err = store.Affected(db.Model(&account).Updates(map[string]any{
		"encrypted_config_json": cipher,
		"config_hash":           hash,
		"config_version":        gorm.Expr("config_version + 1"),
	}))
	if err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			restore := map[string]any{
				"encrypted_config_json": account.EncryptedConfigJSON,
				"config_hash":           account.ConfigHash,
				"config_version":        account.ConfigVersion,
			}
			if rollback := db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Updates(restore).Error; rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider_config.updated", "provider_account", id, nil, "", "")
	return nil
}

// Activate marks a configured provider account active and loads its runtime adapter.
func (s *ProviderAdminService) Activate(ctx context.Context, actor *domain.AdminUser, id string) error {
	var account domain.ProviderAccount
	db := s.db.WithContext(ctx)
	if err := db.First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	if err := store.Affected(db.Model(&account).Update("active", true)); err != nil {
		return err
	}
	if err := s.runtime.Reload(ctx, id); err != nil {
		_ = db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("active", false).Error
		return err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider.activated", "provider_account", id, nil, "", "")
	return nil
}

// Deactivate marks a provider account inactive and removes its runtime adapter.
func (s *ProviderAdminService) Deactivate(ctx context.Context, actor *domain.AdminUser, id string) error {
	if err := store.Affected(s.db.WithContext(ctx).Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("active", false)); err != nil {
		return err
	}
	s.runtime.Disable(id)
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "provider.deactivated", "provider_account", id, nil, "", "")
	return nil
}

func (s *ProviderAdminService) encode(config map[string]any) (string, string, error) {
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
	if providers.String(config, "webhook_secret") == "" {
		return errors.New("webhook_secret is required")
	}
	return nil
}
