package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nyaruka/phonenumbers"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/providers"
	"github.com/momobasehq/momobase/internal/store"
)

type ProviderAdminService struct {
	db        *gorm.DB
	audit     *AuditService
	encryptor *platform.Encryptor
	registry  providers.Registry
	runtime   *ProviderRuntimeManager
}

func NewProviderAdminService(
	db *gorm.DB,
	audit *AuditService,
	enc *platform.Encryptor,
	registry providers.Registry,
	runtime *ProviderRuntimeManager,
) *ProviderAdminService {
	return &ProviderAdminService{db: db, audit: audit, encryptor: enc, registry: registry, runtime: runtime}
}

func (s *ProviderAdminService) CreateAccount(
	actor *domain.AdminUser,
	code string,
	name string,
	environment string,
	countries []string,
	config map[string]any,
) (*domain.ProviderAccount, error) {
	if _, err := s.registry.Create(code, nil); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("provider name is required")
	}
	if environment != "sandbox" && environment != "production" {
		return nil, errors.New("environment must be sandbox or production")
	}
	countries, err := NormalizeProviderCountries(countries)
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
	if err = s.db.Create(account).Error; err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(
		actorID(actor),
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

func (s *ProviderAdminService) UpdateCountries(ctx context.Context, actor *domain.AdminUser, id string, countries []string) error {
	countries, err := NormalizeProviderCountries(countries)
	if err != nil {
		return err
	}
	var account domain.ProviderAccount
	if err = s.db.First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	plain, err := s.runtime.plain(&account)
	if err != nil {
		return err
	}
	if err = validateProviderConfig(plain); err != nil {
		return err
	}
	if err = updateProviderCountries(s.db, id, countries); err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			if rollback := updateProviderCountries(s.db, id, account.Countries); rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(
		actorID(actor),
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

func (s *ProviderAdminService) UpdateConfig(ctx context.Context, actor *domain.AdminUser, id string, config map[string]any) error {
	var account domain.ProviderAccount
	if err := s.db.First(&account, "id = ?", id).Error; err != nil {
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
	err = store.Affected(s.db.Model(&account).Updates(map[string]any{
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
			if rollback := s.db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Updates(restore).Error; rollback != nil {
				return errors.Join(err, rollback)
			}
			return err
		}
	}
	s.audit.RecordBestEffort(actorID(actor), "admin", "provider_config.updated", "provider_account", id, nil, "", "")
	return nil
}

func (s *ProviderAdminService) Activate(ctx context.Context, actor *domain.AdminUser, id string) error {
	var account domain.ProviderAccount
	if err := s.db.First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	if len(account.Countries) == 0 {
		return errors.New("provider must support at least one country before activation")
	}
	if err := store.Affected(s.db.Model(&account).Update("active", true)); err != nil {
		return err
	}
	if err := s.runtime.Reload(ctx, id); err != nil {
		_ = s.db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("active", false).Error
		return err
	}
	s.audit.RecordBestEffort(actorID(actor), "admin", "provider.activated", "provider_account", id, nil, "", "")
	return nil
}

func (s *ProviderAdminService) Deactivate(_ context.Context, actor *domain.AdminUser, id string) error {
	if err := store.Affected(s.db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("active", false)); err != nil {
		return err
	}
	s.runtime.Disable(id)
	s.audit.RecordBestEffort(actorID(actor), "admin", "provider.deactivated", "provider_account", id, nil, "", "")
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

func NormalizeProviderCountries(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one supported country is required")
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		country, err := NormalizeTransactionCountry(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[country]; !exists {
			seen[country] = struct{}{}
			out = append(out, country)
		}
	}
	return out, nil
}

func NormalizeTransactionCountry(country string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 || phonenumbers.GetCountryCodeForRegion(country) == 0 {
		return "", errors.New("country must be a supported ISO-3166 alpha-2 code")
	}
	return country, nil
}

func actorID(actor *domain.AdminUser) string {
	if actor == nil || actor.ID == "" {
		return "system"
	}
	return actor.ID
}
