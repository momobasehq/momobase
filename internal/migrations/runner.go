package migrations

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Run applies every pending migration in ascending version order and returns the
// identifiers it applied. It is idempotent, so it is safe to call on every start.
func Run(ctx context.Context, db *gorm.DB, log *slog.Logger) ([]string, error) {
	return run(ctx, db, log, All())
}

// Pending returns the identifiers of migrations that have not been applied. A
// database with no ledger has every migration pending.
func Pending(ctx context.Context, db *gorm.DB) ([]string, error) {
	applied, err := ledger(ctx, db)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, migration := range ordered(All()) {
		if !applied[migration.ID()] {
			out = append(out, migration.ID())
		}
	}
	return out, nil
}

func run(ctx context.Context, db *gorm.DB, log *slog.Logger, migrations []Migration) ([]string, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := validate(migrations); err != nil {
		return nil, err
	}
	scoped := db.WithContext(ctx)
	if err := scoped.AutoMigrate(&schemaMigration{}); err != nil {
		return nil, fmt.Errorf("create migration ledger: %w", err)
	}
	applied, err := ledger(ctx, db)
	if err != nil {
		return nil, err
	}
	var done []string
	for _, migration := range ordered(migrations) {
		if applied[migration.ID()] {
			continue
		}
		if err = apply(scoped, log, migration); err != nil {
			return done, err
		}
		done = append(done, migration.ID())
	}
	return done, nil
}

// apply records the migration as dirty, runs it, then clears the flag.
//
// The change deliberately does not run inside a transaction. MySQL commits DDL
// implicitly, so a transactional runner would give one driver different recovery
// semantics from the others. Marking first means an interrupted migration blocks
// the next start rather than being retried against a half-changed schema.
func apply(db *gorm.DB, log *slog.Logger, migration Migration) error {
	started := time.Now()
	row := schemaMigration{ID: migration.ID(), AppliedAt: started.UTC(), Dirty: true}
	if err := db.Create(&row).Error; err != nil {
		return fmt.Errorf("record migration %s: %w", migration.ID(), err)
	}
	if err := migration.Up(db); err != nil {
		return fmt.Errorf("apply migration %s: %w", migration.ID(), err)
	}
	elapsed := time.Since(started)
	updates := map[string]any{"dirty": false, "duration_ms": elapsed.Milliseconds()}
	if err := db.Model(&schemaMigration{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("finalize migration %s: %w", migration.ID(), err)
	}
	log.Info(
		"migration applied",
		slog.String("migration", migration.ID()),
		slog.Int64("duration_ms", elapsed.Milliseconds()),
	)
	return nil
}

// ledger returns the applied migration identifiers, refusing to proceed when an
// earlier run was interrupted partway through a migration.
func ledger(ctx context.Context, db *gorm.DB) (map[string]bool, error) {
	scoped := db.WithContext(ctx)
	if !scoped.Migrator().HasTable(&schemaMigration{}) {
		return map[string]bool{}, nil
	}
	var rows []schemaMigration
	if err := scoped.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	applied := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row.Dirty {
			return nil, fmt.Errorf(
				"migration %s did not finish: verify the schema by hand, then delete its row from schema_migrations",
				row.ID,
			)
		}
		applied[row.ID] = true
	}
	return applied, nil
}

func ordered(migrations []Migration) []Migration {
	out := slices.Clone(migrations)
	slices.SortFunc(out, func(a, b Migration) int { return strings.Compare(a.ID(), b.ID()) })
	return out
}
