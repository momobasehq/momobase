package bootstrap

import (
	"context"
	"log/slog"

	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/migrations"
)

// Migrate applies pending versioned migrations and then converges the schema
// with the current domain models.
//
// The order matters. Versioned migrations express the changes AutoMigrate cannot
// — renames, drops, and backfills — and run first, against the schema as it was.
// AutoMigrate then creates and widens whatever the models need. Both steps are
// idempotent, so this is safe to run on every start and against a database
// created by an earlier release.
func Migrate(ctx context.Context, db *gorm.DB, log *slog.Logger) error {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	applied, err := migrations.Run(ctx, db, log)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		log.Info("schema migrations applied", slog.Int("count", len(applied)), slog.Any("migrations", applied))
	}
	return AutoMigrate(db)
}

// WarnPendingMigrations logs the migrations a database still needs when this
// process is configured not to apply them.
//
// It never blocks start-up. Running migrations as a separate pre-deploy step is
// the recommended practice for more than one replica, and refusing to serve would
// turn that deliberate choice into an outage.
func WarnPendingMigrations(ctx context.Context, db *gorm.DB, log *slog.Logger) {
	if log == nil {
		return
	}
	pending, err := migrations.Pending(ctx, db)
	if err != nil {
		log.Warn("cannot read the migration ledger", slog.String("error", err.Error()))
		return
	}
	if len(pending) > 0 {
		log.Warn(
			"schema migrations are pending and AUTO_MIGRATE is disabled; run `momobase migrate`",
			slog.Any("pending", pending),
		)
	}
}

// AutoMigrate converges the database schema with every persistent domain model.
// It creates tables, adds columns, and widens types, but never renames or drops:
// those changes belong in a versioned migration.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&domain.AdminUser{},
		&domain.AdminSession{},
		&domain.Permission{},
		&domain.Role{},
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
}
