package migrations

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

type legacyProviderCountries struct {
	ID        string
	Countries string
}

type providerAccountLocation struct {
	Country  string `gorm:"column:country;size:2;not null;default:''"`
	Currency string `gorm:"column:currency;size:3;index;not null;default:UGX"`
}

func (providerAccountLocation) TableName() string { return "provider_accounts" }

type appCurrency struct {
	Currency string `gorm:"column:currency;size:3;index;not null;default:UGX"`
}

func (appCurrency) TableName() string { return "apps" }

// upAccountLocation replaces provider country lists with one country and pins every
// existing provider and app to UGX. Charge and transaction fee columns are additive,
// so AutoMigrate creates them after this data migration has completed.
func upAccountLocation(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasTable("provider_accounts") {
		return addAppCurrency(migrator)
	}

	backfill := map[string]string{}
	if migrator.HasColumn("provider_accounts", "countries") {
		rows := []legacyProviderCountries{}
		if err := db.Table("provider_accounts").Select("id", "countries").Scan(&rows).Error; err != nil {
			return fmt.Errorf("read provider countries: %w", err)
		}
		for _, row := range rows {
			countries := []string{}
			raw := strings.TrimSpace(row.Countries)
			if raw != "" && raw != "null" {
				if err := json.Unmarshal([]byte(raw), &countries); err != nil {
					return fmt.Errorf("decode countries for provider account %s: %w", row.ID, err)
				}
			}
			if len(countries) == 0 {
				return fmt.Errorf(
					"provider account %s has no country; set its countries column to a one-item JSON array before retrying",
					row.ID,
				)
			}
			backfill[row.ID] = countries[0]
		}
	}

	location := &providerAccountLocation{}
	if !migrator.HasColumn("provider_accounts", "country") {
		if err := migrator.AddColumn(location, "Country"); err != nil {
			return fmt.Errorf("add provider country: %w", err)
		}
	}
	if !migrator.HasColumn("provider_accounts", "currency") {
		if err := migrator.AddColumn(location, "Currency"); err != nil {
			return fmt.Errorf("add provider currency: %w", err)
		}
	}
	for id, country := range backfill {
		if err := db.Table("provider_accounts").Where("id = ?", id).Update("country", country).Error; err != nil {
			return fmt.Errorf("backfill country for provider account %s: %w", id, err)
		}
	}
	if migrator.HasColumn("provider_accounts", "countries") {
		// The identifier is fixed, not caller-controlled. Direct DDL avoids SQLite's
		// GORM migrator rebuilding the table from a partial migration-only struct.
		if err := db.Exec("ALTER TABLE provider_accounts DROP COLUMN countries").Error; err != nil {
			return fmt.Errorf("drop provider countries: %w", err)
		}
	}
	return addAppCurrency(migrator)
}

func addAppCurrency(migrator gorm.Migrator) error {
	if !migrator.HasTable("apps") || migrator.HasColumn("apps", "currency") {
		return nil
	}
	if err := migrator.AddColumn(&appCurrency{}, "Currency"); err != nil {
		return fmt.Errorf("add app currency: %w", err)
	}
	return nil
}
