package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// legacyRow models the shape a table had before a rename, so the runner can be
// exercised against a real column rename rather than a no-op.
type legacyRow struct {
	ID            string `gorm:"primaryKey"`
	CustomerPhone string `gorm:"size:64"`
}

// TableName returns the fixture table name.
func (legacyRow) TableName() string { return "legacy" }

var databases atomic.Int64

// testDatabase opens an isolated in-memory database. Each call gets its own DSN,
// so two databases in one test never share a schema.
func testDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:migrations_%d?mode=memory&cache=shared", databases.Add(1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	return db
}

// renameFixture is a guarded rename, written the way a real migration must be.
func renameFixture() Migration {
	return Migration{Version: "0002", Name: "customer_account", Up: func(db *gorm.DB) error {
		m := db.Migrator()
		if !m.HasTable("legacy") || m.HasColumn("legacy", "customer_account") {
			return nil
		}
		if !m.HasColumn("legacy", "customer_phone") {
			return nil
		}
		return m.RenameColumn("legacy", "customer_phone", "customer_account")
	}}
}

func appliedIDs(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var rows []schemaMigration
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.Dirty {
			t.Errorf("migration %s was left dirty", row.ID)
		}
		out = append(out, row.ID)
	}
	return out
}

func TestRunAppliesPendingMigrationsInOrder(t *testing.T) {
	db := testDatabase(t)
	var order []string
	record := func(id string) func(*gorm.DB) error {
		return func(*gorm.DB) error {
			order = append(order, id)
			return nil
		}
	}
	// Declared out of order to prove the runner sorts by version.
	list := []Migration{
		{Version: "0003", Name: "third", Up: record("third")},
		{Version: "0001", Name: "first", Up: record("first")},
		{Version: "0002", Name: "second", Up: record("second")},
	}

	done, err := run(context.Background(), db, nil, list)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(done) != 3 {
		t.Fatalf("run() applied %v, want three migrations", done)
	}
	if strings.Join(order, ",") != "first,second,third" {
		t.Fatalf("applied in order %v, want ascending version order", order)
	}
	if got := appliedIDs(t, db); strings.Join(got, ",") != "0001_first,0002_second,0003_third" {
		t.Fatalf("ledger = %v, want every migration recorded", got)
	}
}

func TestRunIsIdempotent(t *testing.T) {
	db := testDatabase(t)
	calls := 0
	list := []Migration{{Version: "0001", Name: "once", Up: func(*gorm.DB) error {
		calls++
		return nil
	}}}

	if _, err := run(context.Background(), db, nil, list); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	done, err := run(context.Background(), db, nil, list)
	if err != nil {
		t.Fatalf("second run() error = %v", err)
	}
	if len(done) != 0 {
		t.Fatalf("second run() applied %v, want nothing", done)
	}
	if calls != 1 {
		t.Fatalf("migration ran %d times, want once", calls)
	}
}

// TestRunRenamesAColumnAndPreservesRows is the assertion that matters most: a
// rename that lost data would otherwise look like a successful migration.
func TestRunRenamesAColumnAndPreservesRows(t *testing.T) {
	db := testDatabase(t)
	if err := db.AutoMigrate(&legacyRow{}); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := db.Create(&legacyRow{ID: "txn_1", CustomerPhone: "256770000000"}).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if _, err := run(context.Background(), db, nil, []Migration{renameFixture()}); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	migrator := db.Migrator()
	if migrator.HasColumn("legacy", "customer_phone") {
		t.Error("customer_phone still exists after the rename")
	}
	if !migrator.HasColumn("legacy", "customer_account") {
		t.Fatal("customer_account was not created by the rename")
	}
	var account string
	if err := db.Table("legacy").Where("id = ?", "txn_1").Pluck("customer_account", &account).Error; err != nil {
		t.Fatalf("read renamed column: %v", err)
	}
	if account != "256770000000" {
		t.Fatalf("customer_account = %q, want the value preserved by the rename", account)
	}
}

// TestRunSkipsAMigrationWhoseChangeIsAlreadyPresent covers the property that
// removes the need for a baselining step.
func TestRunSkipsAMigrationWhoseChangeIsAlreadyPresent(t *testing.T) {
	db := testDatabase(t)
	// The table never existed, so the guarded rename has nothing to do.
	done, err := run(context.Background(), db, nil, []Migration{renameFixture()})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(done) != 1 {
		t.Fatalf("run() applied %v, want the migration recorded even though it was a no-op", done)
	}
	if got := appliedIDs(t, db); strings.Join(got, ",") != "0002_customer_account" {
		t.Fatalf("ledger = %v, want the no-op recorded", got)
	}
}

func TestRunRefusesToProceedAfterAnInterruptedMigration(t *testing.T) {
	db := testDatabase(t)
	failing := []Migration{{Version: "0001", Name: "broken", Up: func(*gorm.DB) error {
		return errors.New("upstream schema change failed")
	}}}

	if _, err := run(context.Background(), db, nil, failing); err == nil {
		t.Fatal("run() error = nil, want the migration failure")
	}
	var row schemaMigration
	if err := db.First(&row, "id = ?", "0001_broken").Error; err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if !row.Dirty {
		t.Fatal("a failed migration must stay marked dirty")
	}

	// A later start must refuse rather than run anything against the schema.
	healthy := []Migration{{Version: "0001", Name: "broken", Up: func(*gorm.DB) error { return nil }}}
	_, err := run(context.Background(), db, nil, healthy)
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("run() after a dirty migration = %v, want a refusal", err)
	}
	if _, err = Pending(context.Background(), db); err == nil {
		t.Fatal("Pending() after a dirty migration = nil, want a refusal")
	}
}

func TestRunRejectsAnAmbiguousMigrationList(t *testing.T) {
	noop := func(*gorm.DB) error { return nil }
	tests := map[string][]Migration{
		"missing version": {{Name: "unversioned", Up: noop}},
		"missing name":    {{Version: "0001", Up: noop}},
		"missing up":      {{Version: "0001", Name: "empty"}},
		"duplicate id":    {{Version: "0001", Name: "same", Up: noop}, {Version: "0001", Name: "same", Up: noop}},
		"shared version":  {{Version: "0001", Name: "one", Up: noop}, {Version: "0001", Name: "two", Up: noop}},
	}
	for name, list := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := run(context.Background(), testDatabase(t), nil, list); err == nil {
				t.Fatal("run() error = nil, want a validation error")
			}
		})
	}
}

func TestPendingReportsUnappliedMigrations(t *testing.T) {
	db := testDatabase(t)
	// A database with no ledger has everything pending.
	pending, err := Pending(context.Background(), db)
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if len(pending) != len(All()) {
		t.Fatalf("Pending() = %v, want every migration", pending)
	}

	if _, err = Run(context.Background(), db, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if pending, err = Pending(context.Background(), db); err != nil || len(pending) != 0 {
		t.Fatalf("Pending() after Run() = %v, %v, want none", pending, err)
	}
}

// TestAllIsAValidMigrationList guards the shipped list against the mistakes the
// runner refuses at start-up, such as two developers claiming the same version.
func TestAllIsAValidMigrationList(t *testing.T) {
	if err := validate(All()); err != nil {
		t.Fatalf("validate(All()) error = %v", err)
	}
	for _, migration := range All() {
		if migration.Version != strings.TrimSpace(migration.Version) || len(migration.Version) != 4 {
			t.Errorf("migration %s has a version that is not a four-digit ordinal", migration.ID())
		}
	}
}

// legacyTransaction models the pre-rename transactions table, so the shipped
// migration is exercised against the column it actually renames.
type legacyTransaction struct {
	ID            string `gorm:"primaryKey"`
	CustomerPhone string `gorm:"size:64"`
}

// TableName returns the table the migration targets.
func (legacyTransaction) TableName() string { return "transactions" }

func TestUpCustomerAccountRenamesTheColumn(t *testing.T) {
	db := testDatabase(t)
	if err := db.AutoMigrate(&legacyTransaction{}); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}
	if err := db.Create(&legacyTransaction{ID: "txn_1", CustomerPhone: "256770000000"}).Error; err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if err := upCustomerAccount(db); err != nil {
		t.Fatalf("upCustomerAccount() error = %v", err)
	}
	migrator := db.Migrator()
	if !migrator.HasColumn("transactions", "customer_account") || migrator.HasColumn("transactions", "customer_phone") {
		t.Fatal("upCustomerAccount() did not rename customer_phone to customer_account")
	}
	var account string
	if err := db.Table("transactions").Select("customer_account").Where("id = ?", "txn_1").Scan(&account).Error; err != nil {
		t.Fatalf("read renamed column: %v", err)
	}
	if account != "256770000000" {
		t.Errorf("customer_account = %q, want the value carried over by the rename", account)
	}
	// Running it again must not fail: the guards make an upgrade and a fresh install
	// share one code path.
	if err := upCustomerAccount(db); err != nil {
		t.Fatalf("upCustomerAccount() on an already-renamed table error = %v", err)
	}
}

func TestUpCustomerAccountSkipsADatabaseWithoutTheColumn(t *testing.T) {
	if err := upCustomerAccount(testDatabase(t)); err != nil {
		t.Fatalf("upCustomerAccount() with no transactions table error = %v", err)
	}
}

type legacyProviderAccount struct {
	ID        string `gorm:"primaryKey"`
	Countries string `gorm:"type:text"`
}

func (legacyProviderAccount) TableName() string { return "provider_accounts" }

type legacyApp struct {
	ID string `gorm:"primaryKey"`
}

func (legacyApp) TableName() string { return "apps" }

func TestUpAccountLocationBackfillsFirstCountryAndUGX(t *testing.T) {
	db := testDatabase(t)
	if err := db.AutoMigrate(&legacyProviderAccount{}, &legacyApp{}); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}
	if err := db.Create(&legacyProviderAccount{ID: "pacc_1", Countries: `["RW","UG"]`}).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if err := db.Create(&legacyApp{ID: "app_1"}).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := upAccountLocation(db); err != nil {
		t.Fatalf("upAccountLocation() error = %v", err)
	}

	var provider struct{ Country, Currency string }
	if err := db.Table("provider_accounts").Where("id = ?", "pacc_1").Scan(&provider).Error; err != nil {
		t.Fatalf("read provider location: %v", err)
	}
	if provider.Country != "RW" || provider.Currency != "UGX" {
		t.Errorf("provider location = %+v, want RW/UGX", provider)
	}
	var currency string
	if err := db.Table("apps").Where("id = ?", "app_1").Pluck("currency", &currency).Error; err != nil {
		t.Fatalf("read app currency: %v", err)
	}
	if currency != "UGX" {
		t.Errorf("app currency = %q, want UGX", currency)
	}
	if db.Migrator().HasColumn("provider_accounts", "countries") {
		t.Error("legacy countries column still exists")
	}
}

func TestUpAccountLocationRejectsProviderWithoutCountry(t *testing.T) {
	db := testDatabase(t)
	if err := db.AutoMigrate(&legacyProviderAccount{}); err != nil {
		t.Fatalf("prepare legacy schema: %v", err)
	}
	if err := db.Create(&legacyProviderAccount{ID: "pacc_empty", Countries: `[]`}).Error; err != nil {
		t.Fatalf("seed provider: %v", err)
	}
	if err := upAccountLocation(db); err == nil || !strings.Contains(err.Error(), "pacc_empty has no country") {
		t.Fatalf("upAccountLocation() error = %v, want account guidance", err)
	}
}
