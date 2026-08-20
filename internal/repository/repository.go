// Package repository is the only place in Momobase that reaches the database.
//
// Every persisted entity has one repository, and a repository is the whole of what
// a service may do with that table: the query itself never leaves this package, so a
// service cannot widen a WHERE clause or start a transaction by accident.
//
// A Set is one repository per entity sharing a handle. Within swaps that handle for a
// transaction and hands back a Set bound to it, which is why a multi-table write is
// still one transaction without any caller holding a *gorm.DB.
package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrNotFound reports that a row a caller named does not exist, or that a write
// matched none. Services compare against this rather than the driver's own error, so
// nothing above this package has to import the ORM to interpret a failure.
var ErrNotFound = gorm.ErrRecordNotFound

// IsNotFound reports whether err is, or wraps, ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// Page is one page of rows and the total the page was taken from.
type Page[T any] struct {
	Items []T
	Total int64
}

// base carries the handle a repository queries through. Embedding it is what makes a
// repository transaction-aware: the Set built inside Within hands every repository the
// transaction instead of the pool, so nothing has to be re-bound by hand.
type base[T any] struct{ db *gorm.DB }

func (b base[T]) session(ctx context.Context) *gorm.DB { return b.db.WithContext(ctx) }

// model returns a query already scoped to this repository's table, which is what an
// update or a count needs when there is no row value to infer it from.
func (b base[T]) model(ctx context.Context) *gorm.DB {
	var zero T
	return b.session(ctx).Model(&zero)
}

func (b base[T]) create(ctx context.Context, row *T) error {
	return b.session(ctx).Create(row).Error
}

func (b base[T]) first(ctx context.Context, query string, args ...any) (*T, error) {
	var row T
	if err := b.session(ctx).Where(query, args...).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (b base[T]) find(ctx context.Context, order, query string, args ...any) ([]T, error) {
	rows := []T{}
	session := b.session(ctx)
	if query != "" {
		session = session.Where(query, args...)
	}
	if order != "" {
		session = session.Order(order)
	}
	return rows, session.Find(&rows).Error
}

// update applies values to every matching row and reports ErrNotFound when none match.
// A write that changes nothing is a failure here, never a silent success.
func (b base[T]) update(ctx context.Context, values map[string]any, query string, args ...any) error {
	return affected(b.model(ctx).Where(query, args...).Updates(values))
}

// touch applies values without requiring a match. It exists for the best-effort
// bookkeeping writes — last-used stamps and the like — where no row is not an error.
func (b base[T]) touch(ctx context.Context, values map[string]any, query string, args ...any) error {
	return b.model(ctx).Where(query, args...).Updates(values).Error
}

func (b base[T]) deleteWhere(ctx context.Context, query string, args ...any) error {
	var zero T
	return affected(b.session(ctx).Where(query, args...).Delete(&zero))
}

func (b base[T]) count(ctx context.Context, query string, args ...any) (int64, error) {
	var total int64
	session := b.model(ctx)
	if query != "" {
		session = session.Where(query, args...)
	}
	return total, session.Count(&total).Error
}

// page returns one bounded page and the unpaged total, in that order, so a listing
// reports how much it is a page of.
func (b base[T]) page(ctx context.Context, order string, number, size int) (Page[T], error) {
	total, err := b.count(ctx, "")
	if err != nil {
		return Page[T]{}, err
	}
	items := []T{}
	err = b.session(ctx).Order(order).Limit(size).Offset((number - 1) * size).Find(&items).Error
	return Page[T]{Items: items, Total: total}, err
}

// affected turns a successful write that matched no row into ErrNotFound.
func affected(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
