package migrations

import (
	"fmt"

	"gorm.io/gorm"
)

// upChargeComponents replaces each charge's type and value columns with separate flat and
// percentage columns, so one rule can bill both. A legacy rule becomes the matching half.
func upChargeComponents(db *gorm.DB) error {
	for _, table := range []string{"apps", "provider_accounts"} {
		for _, prefix := range []string{"collection_charge_", "disbursement_charge_"} {
			if err := splitCharge(db, table, prefix); err != nil {
				return fmt.Errorf("split %s.%s*: %w", table, prefix, err)
			}
		}
	}
	return nil
}

// splitCharge migrates one prefix of one table. The identifiers come from fixed lists
// above, never from callers, so building DDL from them is safe.
func splitCharge(db *gorm.DB, table, prefix string) error {
	migrator := db.Migrator()
	if !migrator.HasTable(table) {
		return nil
	}
	typeColumn, valueColumn := prefix+"type", prefix+"value"
	flatColumn, percentageColumn := prefix+"flat", prefix+"percentage"

	// Direct DDL, as in 0003, keeps SQLite's GORM migrator from rebuilding the table.
	for _, column := range []string{flatColumn, percentageColumn} {
		if migrator.HasColumn(table, column) {
			continue
		}
		ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s bigint NOT NULL DEFAULT 0", table, column)
		if err := db.Exec(ddl).Error; err != nil {
			return fmt.Errorf("add %s: %w", column, err)
		}
	}

	if migrator.HasColumn(table, typeColumn) && migrator.HasColumn(table, valueColumn) {
		backfill := fmt.Sprintf(
			"UPDATE %[1]s SET %[2]s = CASE WHEN %[4]s = 'percentage' THEN 0 ELSE %[5]s END, "+
				"%[3]s = CASE WHEN %[4]s = 'percentage' THEN %[5]s ELSE 0 END",
			table, flatColumn, percentageColumn, typeColumn, valueColumn,
		)
		if err := db.Exec(backfill).Error; err != nil {
			return fmt.Errorf("backfill: %w", err)
		}
	}

	for _, column := range []string{typeColumn, valueColumn} {
		if !migrator.HasColumn(table, column) {
			continue
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)).Error; err != nil {
			return fmt.Errorf("drop %s: %w", column, err)
		}
	}
	return nil
}
