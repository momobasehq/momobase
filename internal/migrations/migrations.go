package migrations

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Migration is one ordered schema change.
//
// Up must be safe to run against a database where the change is already present,
// so that a schema created before this ledger existed, or changed out of band, is
// recorded rather than rejected. Reference tables by name rather than by a domain
// model: a migration is a statement about the schema at one point in history, and
// a model reference would silently change meaning when that model next changes.
type Migration struct {
	// Version is the zero-padded ordinal that orders the migration, such as "0002".
	Version string
	// Name describes the change in snake_case, such as "customer_account".
	Name string
	// Up applies the migration.
	Up func(*gorm.DB) error
}

// ID returns the identifier recorded in the ledger, such as "0002_customer_account".
func (m Migration) ID() string {
	return m.Version + "_" + m.Name
}

// schemaMigration is one row of the applied-migration ledger. It is deliberately
// private to this package: it records the migration history rather than any part
// of the business domain.
type schemaMigration struct {
	ID         string    `gorm:"primaryKey;size:96"`
	AppliedAt  time.Time `gorm:"not null"`
	DurationMs int64     `gorm:"not null;default:0"`
	Dirty      bool      `gorm:"not null;default:false"`
}

// TableName returns the ledger's table name.
func (schemaMigration) TableName() string {
	return "schema_migrations"
}

// All returns the migrations that define the schema, in ascending version order.
func All() []Migration {
	return []Migration{
		{Version: "0001", Name: "baseline", Up: upBaseline},
	}
}

// upBaseline records that this ledger governs the database. There is nothing to
// apply: the tables themselves are converged from the domain models by
// bootstrap.AutoMigrate, so a database created by an earlier release and a
// freshly created one are both already correct at this version.
func upBaseline(*gorm.DB) error {
	return nil
}

// validate rejects a migration list that cannot be ordered unambiguously.
func validate(migrations []Migration) error {
	seen := make(map[string]bool, len(migrations))
	versions := make(map[string]string, len(migrations))
	for _, migration := range migrations {
		switch {
		case strings.TrimSpace(migration.Version) == "":
			return fmt.Errorf("migration %q has no version", migration.Name)
		case strings.TrimSpace(migration.Name) == "":
			return fmt.Errorf("migration %q has no name", migration.Version)
		case migration.Up == nil:
			return fmt.Errorf("migration %s has no Up function", migration.ID())
		case seen[migration.ID()]:
			return fmt.Errorf("migration %s is declared more than once", migration.ID())
		}
		if other, exists := versions[migration.Version]; exists {
			return fmt.Errorf("migrations %s and %s share version %s", other, migration.Name, migration.Version)
		}
		seen[migration.ID()], versions[migration.Version] = true, migration.Name
	}
	return nil
}
