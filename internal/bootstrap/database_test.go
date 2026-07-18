package bootstrap

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
)

func TestOpenDatabaseSQLiteAndAutoMigrate(t *testing.T) {
	cfg := Config{DB: DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(t.TempDir(), "nested", "momobase.db"),
	}}
	db, err := OpenDatabase(cfg)
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	t.Cleanup(func() { _ = closeDatabase(db) })

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("second AutoMigrate() error = %v", err)
	}

	models := []any{
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
	}
	for _, model := range models {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("migration did not create table for %T", model)
		}
	}
}

func TestOpenDatabaseRejectsUnsupportedDriver(t *testing.T) {
	_, err := OpenDatabase(Config{DB: DatabaseConfig{Type: "oracle"}})
	if err == nil {
		t.Fatal("OpenDatabase() accepted an unsupported driver")
	}
}

func TestAutoMigrateReturnsClosedDatabaseError(t *testing.T) {
	db, err := OpenDatabase(Config{DB: DatabaseConfig{
		Type: "sqlite",
		Path: filepath.Join(t.TempDir(), "closed.db"),
	}})
	if err != nil {
		t.Fatalf("OpenDatabase() error = %v", err)
	}
	if err := closeDatabase(db); err != nil {
		t.Fatalf("closeDatabase() error = %v", err)
	}
	if err := AutoMigrate(db); err == nil {
		t.Fatal("AutoMigrate() succeeded on a closed database")
	}
}

func TestNewLoggerHonorsLevel(t *testing.T) {
	tests := []struct {
		level        string
		debugEnabled bool
		infoEnabled  bool
	}{
		{"debug", true, true},
		{"info", false, true},
		{"warn", false, false},
		{"error", false, false},
		{"unexpected", false, true},
	}
	for _, test := range tests {
		t.Run(test.level, func(t *testing.T) {
			logger := NewLogger(test.level)
			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != test.debugEnabled {
				t.Fatalf("debug enabled = %v", got)
			}
			if got := logger.Enabled(context.Background(), slog.LevelInfo); got != test.infoEnabled {
				t.Fatalf("info enabled = %v", got)
			}
		})
	}
}
