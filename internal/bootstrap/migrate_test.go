package bootstrap

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/migrations"
)

func migrateTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "momobase.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMigrateCreatesTheSchemaAndLedger(t *testing.T) {
	db := migrateTestDatabase(t)
	if err := Migrate(context.Background(), db, discardLogger()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	migrator := db.Migrator()
	for _, model := range []any{&domain.Transaction{}, &domain.AdminUser{}, &domain.ProviderAccount{}} {
		if !migrator.HasTable(model) {
			t.Errorf("Migrate() did not create the table for %T", model)
		}
	}
	if !migrator.HasTable("schema_migrations") {
		t.Fatal("Migrate() did not create the migration ledger")
	}

	var recorded int64
	if err := db.Table("schema_migrations").Count(&recorded).Error; err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if recorded == 0 {
		t.Fatal("Migrate() recorded no migrations")
	}
}

// TestMigrateIsIdempotent covers the property every start-up relies on.
func TestMigrateIsIdempotent(t *testing.T) {
	db := migrateTestDatabase(t)
	for pass := range 3 {
		if err := Migrate(context.Background(), db, discardLogger()); err != nil {
			t.Fatalf("Migrate() pass %d error = %v", pass+1, err)
		}
	}
	var recorded int64
	if err := db.Table("schema_migrations").Count(&recorded).Error; err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if want := int64(len(migrations.All())); recorded != want {
		t.Fatalf("ledger rows = %d, want one row per shipped migration (%d)", recorded, want)
	}
}

// TestMigrateAdoptsADatabaseCreatedBeforeTheLedgerExisted is the upgrade path:
// releases before versioned migrations created their schema with AutoMigrate
// alone, so those databases arrive with tables and data but no ledger.
func TestMigrateAdoptsADatabaseCreatedBeforeTheLedgerExisted(t *testing.T) {
	db := migrateTestDatabase(t)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	existing := &domain.AdminUser{
		BaseModel: domain.BaseModel{ID: "admin_1"},
		Name:      "Existing",
		Email:     "existing@example.com",
		Role:      "super_admin",
		Status:    "active",
	}
	if err := db.Create(existing).Error; err != nil {
		t.Fatalf("seed pre-existing row: %v", err)
	}

	if err := Migrate(context.Background(), db, discardLogger()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	var found domain.AdminUser
	if err := db.First(&found, "id = ?", "admin_1").Error; err != nil {
		t.Fatalf("pre-existing row was lost: %v", err)
	}
	if found.Email != "existing@example.com" {
		t.Fatalf("pre-existing row = %+v, want it preserved", found)
	}
	var recorded int64
	if err := db.Table("schema_migrations").Count(&recorded).Error; err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if recorded == 0 {
		t.Fatal("Migrate() did not adopt the existing database into the ledger")
	}
}

func TestWarnPendingMigrationsDoesNotChangeTheSchema(t *testing.T) {
	db := migrateTestDatabase(t)
	// Never panics and never migrates: the caller opted out of applying them.
	WarnPendingMigrations(context.Background(), db, discardLogger())
	if db.Migrator().HasTable(&domain.Transaction{}) {
		t.Fatal("WarnPendingMigrations() created the schema, want it left untouched")
	}
	WarnPendingMigrations(context.Background(), db, nil)
}
