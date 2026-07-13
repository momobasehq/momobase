package bootstrap

import (
	"gorm.io/gorm"
	"momobase/internal/domain"
)

func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&domain.AdminUser{}, &domain.AdminSession{}, &domain.AuditLog{}, &domain.App{}, &domain.AppCredential{}, &domain.AppSession{}, &domain.ProviderAccount{}, &domain.ProviderHealthSnapshot{}, &domain.PaymentRoute{}, &domain.Transaction{}, &domain.TransactionAttempt{}, &domain.WebhookEvent{}); err != nil {
		return err
	}
	// Backfill installations created before provider config was folded into provider_accounts.
	if db.Migrator().HasTable("provider_definitions") {
		if err := db.Exec(`UPDATE provider_accounts SET provider_code=(SELECT code FROM provider_definitions WHERE id=provider_accounts.provider_definition_id) WHERE provider_code IS NULL OR provider_code=''`).Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasTable("provider_configs") {
		q := `UPDATE provider_accounts SET encrypted_config_json=(SELECT encrypted_config_json FROM provider_configs WHERE provider_account_id=provider_accounts.id ORDER BY version DESC LIMIT 1), config_hash=(SELECT config_hash FROM provider_configs WHERE provider_account_id=provider_accounts.id ORDER BY version DESC LIMIT 1), config_version=COALESCE((SELECT version FROM provider_configs WHERE provider_account_id=provider_accounts.id ORDER BY version DESC LIMIT 1),1) WHERE encrypted_config_json IS NULL OR encrypted_config_json=''`
		if err := db.Exec(q).Error; err != nil {
			return err
		}
	}
	return nil
}
