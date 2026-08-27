package identity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/momobasehq/momobase/internal/cache"
	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/repository"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/utils"
)

// ErrSystemRole indicates an attempt to change or delete a seeded role.
var ErrSystemRole = errors.New("system roles cannot be changed or deleted")

// AuthzService seeds the permission catalogue and manages roles.
type AuthzService struct {
	repos *repository.UnitOfWork
	audit *audit.Service
	cache *cache.RedisStore
}

// NewAuthzService creates a permission and role service.
func NewAuthzService(
	repos *repository.UnitOfWork,
	audit *audit.Service,
	store *cache.RedisStore,
) *AuthzService {
	return &AuthzService{repos: repos, audit: audit, cache: store}
}

// Seed converges the permission catalogue and the system roles with the code in
// domain.Permissions and domain.SystemRoles. It runs on every boot and is idempotent.
//
// System roles are re-synchronised rather than left alone, which is what lets a
// permission added by a new release reach super_admin without an operator doing
// anything. That is only safe because system roles cannot be edited; a custom role is
// never touched here.
func (s *AuthzService) Seed(ctx context.Context) error {
	for _, definition := range domain.Permissions {
		permission := domain.Permission{
			BaseModel:   domain.BaseModel{ID: platform.NewID("perm")},
			Code:        definition.Code,
			Audience:    definition.Audience,
			Description: definition.Description,
		}
		// Upsert rather than insert, so a re-run updates the description in place
		// instead of failing on the natural key.
		if err := s.repos.Permissions.Upsert(ctx, &permission); err != nil {
			return fmt.Errorf("seed permission %s: %w", definition.Code, err)
		}
	}
	for _, definition := range domain.SystemRoles {
		if err := s.seedRole(ctx, definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *AuthzService) seedRole(ctx context.Context, definition domain.SystemRoleDefinition) error {
	permissions, err := s.resolve(ctx, domain.AudienceAdmin, definition.Permissions())
	if err != nil {
		return fmt.Errorf("seed role %s: %w", definition.Name, err)
	}
	return s.repos.Within(ctx, func(r *repository.Set) error {
		role := domain.Role{
			BaseModel:   domain.BaseModel{ID: platform.NewID("role")},
			Name:        definition.Name,
			Description: definition.Description,
			System:      true,
		}
		if err := r.Roles.Upsert(ctx, &role); err != nil {
			return err
		}
		// Re-read rather than trusting the upsert's in-memory row: on a conflict the
		// stored role keeps the id it was first seeded with, and the association has
		// to attach to that one.
		stored, err := r.Roles.ByName(ctx, definition.Name)
		if err != nil {
			return err
		}
		return r.Roles.ReplacePermissions(ctx, stored, permissions)
	})
}

// resolve maps permission codes onto catalogue rows for one audience. The wildcard is
// stored as a row of its own so a role can hold it like any other permission.
func (s *AuthzService) resolve(ctx context.Context, audience string, codes []string) ([]domain.Permission, error) {
	if slices.Contains(codes, domain.PermissionWildcard) {
		wildcard := domain.Permission{
			BaseModel:   domain.BaseModel{ID: platform.NewID("perm")},
			Code:        domain.PermissionWildcard,
			Audience:    audience,
			Description: "Every permission, including ones added by later releases",
		}
		if err := s.repos.Permissions.Upsert(ctx, &wildcard); err != nil {
			return nil, err
		}
		stored, err := s.repos.Permissions.ByCode(ctx, audience, domain.PermissionWildcard)
		if err != nil {
			return nil, err
		}
		return []domain.Permission{*stored}, nil
	}
	if len(codes) == 0 {
		return nil, nil
	}
	permissions, err := s.repos.Permissions.ByCodes(ctx, audience, codes)
	if err != nil {
		return nil, err
	}
	// Refuse the whole set rather than silently granting a subset: a role that quietly
	// dropped a misspelled permission would look correct and authorize less than asked.
	if len(permissions) != len(codes) {
		found := make([]string, 0, len(permissions))
		for _, permission := range permissions {
			found = append(found, permission.Code)
		}
		for _, code := range codes {
			if !slices.Contains(found, code) {
				return nil, fmt.Errorf("unknown %s permission %q", audience, code)
			}
		}
	}
	return permissions, nil
}

// EffectivePermissions returns the permission codes a role grants. An unknown role
// resolves to none, so an administrator whose role was removed fails closed.
func (s *AuthzService) EffectivePermissions(ctx context.Context, roleName string) ([]string, error) {
	role, err := s.repos.Roles.WithPermissions(ctx, roleName)
	if repository.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0, len(role.Permissions))
	for _, permission := range role.Permissions {
		codes = append(codes, permission.Code)
	}
	return codes, nil
}

// ListPermissions returns the seeded catalogue, optionally for one audience only.
func (s *AuthzService) ListPermissions(ctx context.Context, audience string) ([]domain.Permission, error) {
	if audience != "" && audience != domain.AudienceAdmin && audience != domain.AudienceApp {
		return nil, errors.New("audience must be admin or app")
	}
	key := permissionCacheKey(audience)
	if permissions := cache.Get[[]domain.Permission](ctx, s.cache, key); permissions != nil {
		return *permissions, nil
	}
	permissions, err := s.repos.Permissions.List(ctx, audience)
	if err != nil {
		return nil, err
	}
	cache.Set(ctx, s.cache, key, permissions)
	return permissions, nil
}

func permissionCacheKey(audience string) string {
	if audience == "" {
		return "permissions:v1:all"
	}
	return "permissions:v1:" + audience
}

// ListRoles returns every role with its permissions, system roles first.
func (s *AuthzService) ListRoles(ctx context.Context) ([]domain.Role, error) {
	key := roleCacheKey()
	if roles := cache.Get[[]domain.Role](ctx, s.cache, key); roles != nil {
		return *roles, nil
	}
	roles, err := s.repos.Roles.List(ctx)
	if err != nil {
		return nil, err
	}
	cache.Set(ctx, s.cache, key, roles)
	return roles, nil
}

func roleCacheKey() string { return "roles:v1:all" }

// CreateRole persists a custom role holding the supplied permissions.
func (s *AuthzService) CreateRole(
	ctx context.Context,
	actor *domain.AdminUser,
	name, description string,
	codes []string,
) (*domain.Role, error) {
	name = normalizeRoleName(name)
	if name == "" || !utils.ValidIdentifier(name) {
		return nil, errors.New("role name is required and may contain only letters, digits, and _-. and must not exceed 64 characters")
	}
	// A custom role must not shadow a seeded one: AdminUser.Role refers to a role by
	// name, so two rows with one name would make an administrator's access ambiguous.
	if slices.ContainsFunc(domain.SystemRoles, func(d domain.SystemRoleDefinition) bool { return d.Name == name }) {
		return nil, ErrSystemRole
	}
	permissions, err := s.resolve(ctx, domain.AudienceAdmin, codes)
	if err != nil {
		return nil, err
	}
	role := &domain.Role{
		BaseModel:   domain.BaseModel{ID: platform.NewID("role")},
		Name:        name,
		Description: strings.TrimSpace(description),
		System:      false,
		Permissions: permissions,
	}
	roles := []domain.Role{}
	if err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Roles.Create(ctx, role); err != nil {
			return err
		}
		var err error
		roles, err = r.Roles.List(ctx)
		return err
	}); err != nil {
		return nil, err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "role.created", "role", role.ID, map[string]any{"permissions": codes}, "", "")
	cache.Set(ctx, s.cache, roleCacheKey(), roles)
	return role, nil
}

// UpdateRole replaces a custom role's description and permission set.
func (s *AuthzService) UpdateRole(
	ctx context.Context,
	actor *domain.AdminUser,
	name, description string,
	codes []string,
) error {
	role, err := s.customRole(ctx, name)
	if err != nil {
		return err
	}
	permissions, err := s.resolve(ctx, domain.AudienceAdmin, codes)
	if err != nil {
		return err
	}
	roles := []domain.Role{}
	if err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Roles.SetDescription(ctx, role.ID, strings.TrimSpace(description)); err != nil {
			return err
		}
		if err := r.Roles.ReplacePermissions(ctx, role, permissions); err != nil {
			return err
		}
		var err error
		roles, err = r.Roles.List(ctx)
		return err
	}); err != nil {
		return err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "role.updated", "role", role.ID, map[string]any{"permissions": codes}, "", "")
	cache.Set(ctx, s.cache, roleCacheKey(), roles)
	return nil
}

// DeleteRole removes a custom role that no administrator still holds.
func (s *AuthzService) DeleteRole(ctx context.Context, actor *domain.AdminUser, name string) error {
	role, err := s.customRole(ctx, name)
	if err != nil {
		return err
	}
	// Deleting a role an administrator still holds would leave them with no
	// permissions at all, which reads as a broken account rather than a revoked one.
	holders, err := s.repos.AdminUsers.CountWithRole(ctx, role.Name)
	if err != nil {
		return err
	}
	if holders > 0 {
		return fmt.Errorf("role %q is still assigned to %d administrator(s)", role.Name, holders)
	}
	roles := []domain.Role{}
	if err := s.repos.Within(ctx, func(r *repository.Set) error {
		if err := r.Roles.Delete(ctx, role); err != nil {
			return err
		}
		var err error
		roles, err = r.Roles.List(ctx)
		return err
	}); err != nil {
		return err
	}
	s.audit.RecordBestEffort(ctx, actor.ActorID(), "admin", "role.deleted", "role", role.ID, nil, "", "")
	cache.Set(ctx, s.cache, roleCacheKey(), roles)
	return nil
}

// customRole loads a role by name and refuses a seeded one.
func (s *AuthzService) customRole(ctx context.Context, name string) (*domain.Role, error) {
	role, err := s.repos.Roles.ByName(ctx, normalizeRoleName(name))
	if err != nil {
		return nil, err
	}
	if role.System {
		return nil, ErrSystemRole
	}
	return role, nil
}

// RoleExists reports whether a role name can be assigned to an administrator.
func (s *AuthzService) RoleExists(ctx context.Context, name string) (bool, error) {
	return s.repos.Roles.Exists(ctx, normalizeRoleName(name))
}

// ValidateAppScopes checks a credential's requested scopes against the catalogue and
// returns them normalized, so a typo fails at creation rather than at the first
// payment the credential is used for.
func ValidateAppScopes(scopes string) (string, error) {
	fields := strings.Fields(strings.ToLower(scopes))
	if len(fields) == 0 {
		return "", errors.New("at least one scope is required")
	}
	allowed := domain.AppPermissionCodes()
	out := make([]string, 0, len(fields))
	for _, scope := range fields {
		if scope != domain.PermissionWildcard && !slices.Contains(allowed, scope) {
			return "", fmt.Errorf("unknown scope %q", scope)
		}
		if !slices.Contains(out, scope) {
			out = append(out, scope)
		}
	}
	return strings.Join(out, " "), nil
}

func normalizeRoleName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
