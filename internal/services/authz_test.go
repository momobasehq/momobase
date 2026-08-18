package services

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
)

func TestSeedIsIdempotentAndCoversTheCatalogue(t *testing.T) {
	s := stack(t)
	// stack() already seeded once; a second run must converge rather than duplicate.
	noError(s.authz.Seed(context.Background()))

	permissions := must(s.authz.ListPermissions(context.Background(), ""))
	// The wildcard is stored as a row of its own so a role can hold it like any other
	// permission, so the catalogue is the definitions plus that one.
	if len(permissions) != len(domain.Permissions)+1 {
		t.Errorf("catalogue has %d permissions, want %d", len(permissions), len(domain.Permissions)+1)
	}

	roles := must(s.authz.ListRoles(context.Background()))
	if len(roles) != len(domain.SystemRoles) {
		t.Fatalf("seeded %d roles, want %d", len(roles), len(domain.SystemRoles))
	}
	for _, role := range roles {
		if !role.System {
			t.Errorf("seeded role %q is not marked system", role.Name)
		}
	}
}

func TestSeededRolePermissions(t *testing.T) {
	s := stack(t)
	ctx := context.Background()

	// super_admin holds the wildcard rather than an enumerated set, which is what keeps
	// it correct when a later release adds a permission.
	super := must(s.authz.EffectivePermissions(ctx, domain.RoleSuperAdmin))
	if len(super) != 1 || super[0] != domain.PermissionWildcard {
		t.Errorf("super_admin = %v, want just the wildcard", super)
	}

	// operations reproduces what the old hardcoded checks allowed: every read, plus
	// balances, and no mutation at all.
	operations := must(s.authz.EffectivePermissions(ctx, domain.RoleOperations))
	if !slices.Contains(operations, "balances:read") {
		t.Error("operations is missing balances:read")
	}
	for _, permission := range operations {
		if !strings.HasSuffix(permission, ":read") {
			t.Errorf("operations holds %q, which is not a read permission", permission)
		}
	}

	// read_only is operations without the one read that reaches a provider's API.
	readOnly := must(s.authz.EffectivePermissions(ctx, domain.RoleReadOnly))
	if slices.Contains(readOnly, "balances:read") {
		t.Error("read_only must not hold balances:read")
	}
	if len(readOnly) != len(operations)-1 {
		t.Errorf("read_only has %d permissions, want one fewer than operations (%d)", len(readOnly), len(operations))
	}
}

// TestEffectivePermissionsFailsClosed is the safety property: an administrator whose
// role no longer exists must authorize nothing rather than everything.
func TestEffectivePermissionsFailsClosed(t *testing.T) {
	s := stack(t)
	if permissions := must(s.authz.EffectivePermissions(context.Background(), "deleted_role")); len(permissions) != 0 {
		t.Errorf("EffectivePermissions(unknown) = %v, want none", permissions)
	}
}

func TestCustomRoleLifecycle(t *testing.T) {
	s := stack(t)
	ctx := context.Background()

	role := must(s.authz.CreateRole(ctx, s.actor, " Support ", "Reads transactions", []string{"transactions:read", "apps:read"}))
	if role.Name != "support" || role.System {
		t.Fatalf("CreateRole() = %+v, want a normalized non-system role", role)
	}
	if permissions := must(s.authz.EffectivePermissions(ctx, "support")); len(permissions) != 2 {
		t.Errorf("support holds %v, want two permissions", permissions)
	}

	noError(s.authz.UpdateRole(ctx, s.actor, "support", "Reads only transactions", []string{"transactions:read"}))
	if permissions := must(s.authz.EffectivePermissions(ctx, "support")); len(permissions) != 1 || permissions[0] != "transactions:read" {
		t.Errorf("after update support holds %v, want just transactions:read", permissions)
	}

	noError(s.authz.DeleteRole(ctx, s.actor, "support"))
	if permissions := must(s.authz.EffectivePermissions(ctx, "support")); len(permissions) != 0 {
		t.Errorf("deleted role still grants %v", permissions)
	}
}

func TestRoleGuards(t *testing.T) {
	s := stack(t)
	ctx := context.Background()

	t.Run("a system role cannot be shadowed, edited, or deleted", func(t *testing.T) {
		if _, err := s.authz.CreateRole(ctx, s.actor, domain.RoleOperations, "", nil); !errors.Is(err, ErrSystemRole) {
			t.Errorf("CreateRole(operations) error = %v, want %v", err, ErrSystemRole)
		}
		if err := s.authz.UpdateRole(ctx, s.actor, domain.RoleSuperAdmin, "", nil); !errors.Is(err, ErrSystemRole) {
			t.Errorf("UpdateRole(super_admin) error = %v, want %v", err, ErrSystemRole)
		}
		if err := s.authz.DeleteRole(ctx, s.actor, domain.RoleReadOnly); !errors.Is(err, ErrSystemRole) {
			t.Errorf("DeleteRole(read_only) error = %v, want %v", err, ErrSystemRole)
		}
	})

	// A misspelled permission must fail the whole role: silently granting the subset
	// would produce a role that looks right and authorizes less than it claims.
	t.Run("an unknown permission is refused outright", func(t *testing.T) {
		if _, err := s.authz.CreateRole(ctx, s.actor, "typo", "", []string{"transactions:read", "transaction:reed"}); err == nil {
			t.Error("CreateRole() accepted an unknown permission")
		}
		if exists := must(s.authz.RoleExists(ctx, "typo")); exists {
			t.Error("the rejected role was persisted anyway")
		}
	})

	// An app-audience code must not be grantable to an administrator, or the two
	// namespaces would collapse into one.
	t.Run("an app permission is not an admin permission", func(t *testing.T) {
		if _, err := s.authz.CreateRole(ctx, s.actor, "wrongaudience", "", []string{"collections:create"}); err == nil {
			t.Error("CreateRole() granted an app-audience permission to a role")
		}
	})

	t.Run("a role still held cannot be deleted", func(t *testing.T) {
		must(s.authz.CreateRole(ctx, s.actor, "held", "", []string{"apps:read"}))
		noError(s.db.Create(&domain.AdminUser{
			BaseModel: domain.BaseModel{ID: "admin-held"},
			Name:      "Holder",
			Email:     "holder@example.com",
			Role:      "held",
			Status:    "active",
		}).Error)
		if err := s.authz.DeleteRole(ctx, s.actor, "held"); err == nil {
			t.Error("DeleteRole() removed a role an administrator still holds")
		}
	})
}

func TestValidateAppScopes(t *testing.T) {
	for _, scopes := range []string{"transactions:read", "*", " COLLECTIONS:CREATE  transactions:read "} {
		if _, err := ValidateAppScopes(scopes); err != nil {
			t.Errorf("ValidateAppScopes(%q) error = %v", scopes, err)
		}
	}
	if got := must(ValidateAppScopes("transactions:read transactions:read")); got != "transactions:read" {
		t.Errorf("ValidateAppScopes() = %q, want duplicates collapsed", got)
	}
	for name, scopes := range map[string]string{
		"empty":        "   ",
		"typo":         "transaction:reed",
		"admin scope":  "users:create",
		"partly wrong": "transactions:read users:create",
	} {
		if _, err := ValidateAppScopes(scopes); err == nil {
			t.Errorf("ValidateAppScopes(%s) = nil, want an error", name)
		}
	}
}

func TestListPermissionsFiltersByAudience(t *testing.T) {
	s := stack(t)
	app := must(s.authz.ListPermissions(context.Background(), domain.AudienceApp))
	for _, permission := range app {
		if permission.Audience != domain.AudienceApp {
			t.Errorf("audience filter returned %+v", permission)
		}
	}
	if _, err := s.authz.ListPermissions(context.Background(), "nobody"); err == nil {
		t.Error("ListPermissions(nobody) = nil, want an error")
	}
}

func TestChangeRole(t *testing.T) {
	s := stack(t)
	ctx := context.Background()
	users := NewAdminUserService(s.db, NewAuditService(s.db, nil), s.authz)

	target := must(users.Create(ctx, s.actor, "Target", "target@example.com", "password123", domain.RoleReadOnly))

	t.Run("reassigns to any existing role, seeded or custom", func(t *testing.T) {
		must(s.authz.CreateRole(ctx, s.actor, "support", "", []string{"transactions:read"}))
		noError(users.ChangeRole(ctx, s.actor, target.ID, "support"))

		var stored domain.AdminUser
		noError(s.db.First(&stored, "id = ?", target.ID).Error)
		if stored.Role != "support" {
			t.Fatalf("role = %q, want support", stored.Role)
		}
		// The point of resolving permissions per request: the new role takes effect
		// without revoking anything, so no session bookkeeping is involved.
		if permissions := must(s.authz.EffectivePermissions(ctx, stored.Role)); len(permissions) != 1 {
			t.Errorf("effective permissions = %v, want the new role's single permission", permissions)
		}
	})

	// Both a lockout risk — the last super_admin demoting itself leaves nobody able to
	// undo it — and a privilege escalation, since users:update would otherwise be enough
	// to promote yourself.
	t.Run("refuses a self change", func(t *testing.T) {
		if err := users.ChangeRole(ctx, s.actor, s.actor.ID, domain.RoleReadOnly); err == nil {
			t.Error("ChangeRole() let an administrator change their own role")
		}
	})

	t.Run("refuses an unknown role and an unknown user", func(t *testing.T) {
		if err := users.ChangeRole(ctx, s.actor, target.ID, "not_a_role"); err == nil {
			t.Error("ChangeRole() accepted an unknown role")
		}
		if err := users.ChangeRole(ctx, s.actor, "missing", domain.RoleReadOnly); err == nil {
			t.Error("ChangeRole() accepted an unknown administrator")
		}
	})
}
