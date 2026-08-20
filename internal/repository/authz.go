package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/momobasehq/momobase/internal/domain"
)

// PermissionRepo reads and writes the permission catalogue.
//
// It keeps its own handle alongside the embedded base because upserting on a natural
// key and replacing an association are ORM features with no equivalent in the shared
// helpers, and both belong on this side of the boundary rather than in a service.
type PermissionRepo struct {
	base[domain.Permission]
	db *gorm.DB
}

// Upsert stores a catalogue entry, conflicting on the natural key rather than the
// generated ID, so re-seeding updates the description in place instead of failing or
// accumulating duplicates.
func (r PermissionRepo) Upsert(ctx context.Context, permission *domain.Permission) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "code"}, {Name: "audience"}},
		DoUpdates: clause.AssignmentColumns([]string{"description", "updated_at"}),
	}).Create(permission).Error
}

// ByCode returns one catalogue entry.
func (r PermissionRepo) ByCode(ctx context.Context, audience, code string) (*domain.Permission, error) {
	return r.first(ctx, "code = ? AND audience = ?", code, audience)
}

// ByCodes returns the catalogue entries matching codes within one audience. A code with
// no row is simply absent, so the caller can compare counts and refuse the whole set.
func (r PermissionRepo) ByCodes(ctx context.Context, audience string, codes []string) ([]domain.Permission, error) {
	return r.find(ctx, "", "audience = ? AND code IN ?", audience, codes)
}

// List returns the catalogue, optionally narrowed to one audience.
func (r PermissionRepo) List(ctx context.Context, audience string) ([]domain.Permission, error) {
	if audience == "" {
		return r.find(ctx, "audience asc, code asc", "")
	}
	return r.find(ctx, "audience asc, code asc", "audience = ?", audience)
}

// RoleRepo reads and writes roles and the permissions attached to them.
type RoleRepo struct {
	base[domain.Role]
	db *gorm.DB
}

// Create stores a new role.
func (r RoleRepo) Create(ctx context.Context, role *domain.Role) error { return r.create(ctx, role) }

// Upsert stores a role, conflicting on its name so re-seeding a system role updates it
// rather than failing.
func (r RoleRepo) Upsert(ctx context.Context, role *domain.Role) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"description", "system", "updated_at"}),
	}).Create(role).Error
}

// ByName returns a role without its permissions.
func (r RoleRepo) ByName(ctx context.Context, name string) (*domain.Role, error) {
	return r.first(ctx, "name = ?", name)
}

// WithPermissions returns a role and the permissions attached to it, which is what an
// authorization check resolves against.
func (r RoleRepo) WithPermissions(ctx context.Context, name string) (*domain.Role, error) {
	var role domain.Role
	if err := r.db.WithContext(ctx).Preload("Permissions").
		Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// List returns every role with its permissions, system roles first.
func (r RoleRepo) List(ctx context.Context) ([]domain.Role, error) {
	roles := []domain.Role{}
	return roles, r.db.WithContext(ctx).Preload("Permissions").
		Order("system desc, name asc").Find(&roles).Error
}

// Exists reports whether a role of that name is defined.
func (r RoleRepo) Exists(ctx context.Context, name string) (bool, error) {
	count, err := r.count(ctx, "name = ?", name)
	return count > 0, err
}

// SetDescription changes a role's description.
func (r RoleRepo) SetDescription(ctx context.Context, id, description string) error {
	return r.update(ctx, map[string]any{"description": description}, "id = ?", id)
}

// ReplacePermissions makes permissions the role's complete set, dropping whatever it
// held before. Replacing rather than adding is what keeps an update authoritative.
func (r RoleRepo) ReplacePermissions(
	ctx context.Context,
	role *domain.Role,
	permissions []domain.Permission,
) error {
	return r.db.WithContext(ctx).Model(role).Association("Permissions").Replace(permissions)
}

// Delete removes a role and detaches its permissions first, so no join row outlives it.
func (r RoleRepo) Delete(ctx context.Context, role *domain.Role) error {
	if err := r.db.WithContext(ctx).Model(role).Association("Permissions").Clear(); err != nil {
		return err
	}
	return r.deleteWhere(ctx, "id = ?", role.ID)
}
