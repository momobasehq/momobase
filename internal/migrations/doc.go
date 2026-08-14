// Package migrations applies ordered schema changes that GORM's AutoMigrate
// cannot express.
//
// AutoMigrate converges a database toward the current models: it creates tables,
// adds columns, and widens types. It never renames or drops anything, so a
// rename would silently leave the old column behind holding all the data. This
// package covers exactly that gap, and the two run together — versioned
// migrations first, then convergence.
//
// Migrations are Go functions rather than SQL files. They use the driver-portable
// [gorm.io/gorm.Migrator] API, so one definition works across SQLite, PostgreSQL,
// and MySQL, which is what a column rename needs most.
//
// Every migration must be a no-op when its change is already present. That single
// rule is what removes the need for a baselining step: a database created by an
// earlier release, before this ledger existed, is indistinguishable from one that
// has already been migrated.
package migrations
