package store

import (
	"context"
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type testRecord struct {
	ID   int `gorm:"primaryKey"`
	Name string
}

func testDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&testRecord{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func TestWithinCommitsAndRollsBack(t *testing.T) {
	db := testDatabase(t)
	if err := Within(context.Background(), db, func(tx *gorm.DB) error {
		return tx.Create(&testRecord{ID: 1, Name: "committed"}).Error
	}); err != nil {
		t.Fatalf("Within(commit) error = %v", err)
	}
	var count int64
	if err := db.Model(&testRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("committed count = %d, %v", count, err)
	}

	wantErr := errors.New("rollback")
	err := Within(context.Background(), db, func(tx *gorm.DB) error {
		if err := tx.Create(&testRecord{ID: 2, Name: "rolled back"}).Error; err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Within(rollback) error = %v", err)
	}
	if err := db.Model(&testRecord{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count after rollback = %d, %v", count, err)
	}
}

func TestAffected(t *testing.T) {
	db := testDatabase(t)
	if err := db.Create(&testRecord{ID: 1, Name: "one"}).Error; err != nil {
		t.Fatalf("create record: %v", err)
	}
	if err := Affected(db.Model(&testRecord{}).Where("id = ?", 1).Update("name", "updated")); err != nil {
		t.Fatalf("Affected(updated) error = %v", err)
	}
	if err := Affected(db.Model(&testRecord{}).Where("id = ?", 99).Update("name", "missing")); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Affected(missing) error = %v", err)
	}
	wantErr := errors.New("write failed")
	if err := Affected(&gorm.DB{Error: wantErr}); !errors.Is(err, wantErr) {
		t.Fatalf("Affected(error) = %v", err)
	}
}
