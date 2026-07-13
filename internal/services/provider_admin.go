package services

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"momobase/internal/domain"
	"momobase/internal/platform"
	"momobase/internal/providers"
	"momobase/internal/store"
)

type ProviderAdminService struct {
	db        *gorm.DB
	audit     *AuditService
	encryptor *platform.Encryptor
	registry  providers.Registry
	runtime   *ProviderRuntimeManager
}

func NewProviderAdminService(db *gorm.DB, audit *AuditService, enc *platform.Encryptor, registry providers.Registry, runtime *ProviderRuntimeManager) *ProviderAdminService {
	return &ProviderAdminService{db: db, audit: audit, encryptor: enc, registry: registry, runtime: runtime}
}
func (s *ProviderAdminService) CreateAccount(actor *domain.AdminUser, code, name, environment, country string, config map[string]any) (*domain.ProviderAccount, error) {
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
	country, err := NormalizeProviderCountry(country)
	if err != nil {
		return nil, err
	}
	if err = validateProviderShape(country, config); err != nil {
		return nil, err
	}
	cipher, hash, err := s.encode(config)
	if err != nil {
		return nil, err
	}
	account := &domain.ProviderAccount{BaseModel: domain.BaseModel{ID: platform.NewID("pacc")}, ProviderCode: code, Name: name, Environment: environment, Country: country, ConfigVersion: 1, EncryptedConfigJSON: cipher, ConfigHash: hash}
	if err = s.db.Create(account).Error; err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(actorID(actor), "admin", "provider_account.created", "provider_account", account.ID, map[string]any{"provider_code": code, "country": country}, "", "")
	return account, nil
}
func (s *ProviderAdminService) UpdateConfig(ctx context.Context, actor *domain.AdminUser, id string, config map[string]any) error {
	var account domain.ProviderAccount
	if err := s.db.First(&account, "id = ?", id).Error; err != nil {
		return err
	}
	if err := validateProviderShape(account.Country, config); err != nil {
		return err
	}
	cipher, hash, err := s.encode(config)
	if err != nil {
		return err
	}
	err = store.Affected(s.db.Model(&account).Updates(map[string]any{"encrypted_config_json": cipher, "config_hash": hash, "config_version": gorm.Expr("config_version + 1")}))
	if err != nil {
		return err
	}
	if account.Active {
		if err = s.runtime.Reload(ctx, id); err != nil {
			restore := map[string]any{"encrypted_config_json": account.EncryptedConfigJSON, "config_hash": account.ConfigHash, "config_version": account.ConfigVersion}
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
	if err := store.Affected(s.db.Model(&domain.ProviderAccount{}).Where("id = ?", id).Update("active", true)); err != nil {
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
func validateProviderShape(country string, config map[string]any) error {
	if len(config) == 0 {
		return errors.New("provider config is required")
	}
	if providers.String(config, "webhook_secret") == "" {
		return errors.New("webhook_secret is required")
	}
	configured := strings.ToUpper(providers.String(config, "country"))
	if configured != "" && country != domain.CountryGlobal && configured != country {
		return errors.New("provider config country conflicts with account country")
	}
	if country == domain.CountryGlobal && !providers.Bool(config, "supports_global") {
		return errors.New("GLOBAL provider accounts require supports_global=true")
	}
	return nil
}

func NormalizeProviderCountry(country string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		country = domain.CountryGlobal
	}
	if country == domain.CountryGlobal {
		return country, nil
	}
	if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		return "", errors.New("country must be ISO-3166 alpha-2 or GLOBAL")
	}
	return country, nil
}
func NormalizeTransactionCountry(country string) (string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if len(country) != 2 || country[0] < 'A' || country[0] > 'Z' || country[1] < 'A' || country[1] > 'Z' {
		return "", errors.New("country must be ISO-3166 alpha-2")
	}
	return country, nil
}

func actorID(actor *domain.AdminUser) string {
	if actor == nil || actor.ID == "" {
		return "system"
	}
	return actor.ID
}
