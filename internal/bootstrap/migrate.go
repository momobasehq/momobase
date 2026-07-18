package bootstrap

import (
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
)

func AutoMigrate(db *gorm.DB) error {
	err := db.AutoMigrate(
		&domain.AdminUser{},
		&domain.AdminSession{},
		&domain.AuditLog{},
		&domain.App{},
		&domain.AppCredential{},
		&domain.AppSession{},
		&domain.ProviderAccount{},
		&domain.ProviderHealthSnapshot{},
		&domain.PaymentRoute{},
		&domain.Transaction{},
		&domain.TransactionAttempt{},
		&domain.WebhookEvent{},
	)
	if err != nil {
		return err
	}
	return nil
}
