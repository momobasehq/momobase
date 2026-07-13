package store

import (
	"context"

	"gorm.io/gorm"
)

// Within is the only business-layer transaction boundary.
func Within(ctx context.Context, db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}

// Affected converts a successful zero-row write into not-found.
func Affected(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
