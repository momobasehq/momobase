package identity_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/service/audit"
	"github.com/momobasehq/momobase/internal/service/identity"
	"github.com/momobasehq/momobase/internal/testsupport"
)

func TestSeedIsIdempotentAndCoversTheCatalogue(t *testing.T) {
	s := testsupport.New(t)
	// stack() already seeded once; a second run must converge rather than duplicate.
	testsupport.NoError(s.Authz.Seed(context.Background()))

	permissions := testsupport.Must(s.Authz.ListPermissions(context.Background(), ""))
	// The wildcard is stored as a row of its own so a role can hold it like any other
	// permission, so the catalogue is the definitions plus that one.
	if len(permissions) != len(domain.Permissions)+1 {
		t.Errorf("catalogue has %d permissions, want %d", len(permissions), len(domain.Permissions)+1)
	}

	roles := testsupport.Must(s.Authz.ListRoles(context.Background()))
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
	s := testsupport.New(t)
	ctx := context.Background()

	// super_admin holds the wildcard rather than an enumerated set, which is what keeps
	// it correct when a later release adds a permission.
	super := testsupport.Must(s.Authz.EffectivePermissions(ctx, domain.RoleSuperAdmin))
	if len(super) != 1 || super[0] != domain.PermissionWildcard {
		t.Errorf("super_admin = %v, want just the wildcard", super)
	}

	// operations reproduces what the old hardcoded checks allowed: every read, plus
	// balances, and no mutation at all.
	operations := testsupport.Must(s.Authz.EffectivePermissions(ctx, domain.RoleOperations))
	if !slices.Contains(operations, "balances:read") {
		t.Error("operations is missing balances:read")
	}
	for _, permission := range operations {
		if !strings.HasSuffix(permission, ":read") {
			t.Errorf("operations holds %q, which is not a read permission", permission)
		}
	}

	// read_only is operations without the one read that reaches a provider's API.
	readOnly := testsupport.Must(s.Authz.EffectivePermissions(ctx, domain.RoleReadOnly))
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
	s := testsupport.New(t)
	if permissions := testsupport.Must(s.Authz.EffectivePermissions(context.Background(), "deleted_role")); len(permissions) != 0 {
		t.Errorf("EffectivePermissions(unknown) = %v, want none", permissions)
	}
}

func TestCustomRoleLifecycle(t *testing.T) {
	s := testsupport.New(t)
	ctx := context.Background()

	role := testsupport.Must(s.Authz.CreateRole(ctx, s.Actor, " Support ", "Reads transactions", []string{"transactions:read", "apps:read"}))
	if role.Name != "support" || role.System {
		t.Fatalf("CreateRole() = %+v, want a normalized non-system role", role)
	}
	if permissions := testsupport.Must(s.Authz.EffectivePermissions(ctx, "support")); len(permissions) != 2 {
		t.Errorf("support holds %v, want two permissions", permissions)
	}

	testsupport.NoError(s.Authz.UpdateRole(ctx, s.Actor, "support", "Reads only transactions", []string{"transactions:read"}))
	if permissions := testsupport.Must(s.Authz.EffectivePermissions(ctx, "support")); len(permissions) != 1 || permissions[0] != "transactions:read" {
		t.Errorf("after update support holds %v, want just transactions:read", permissions)
	}

	testsupport.NoError(s.Authz.DeleteRole(ctx, s.Actor, "support"))
	if permissions := testsupport.Must(s.Authz.EffectivePermissions(ctx, "support")); len(permissions) != 0 {
		t.Errorf("deleted role still grants %v", permissions)
	}
}

func TestRoleGuards(t *testing.T) {
	s := testsupport.New(t)
	ctx := context.Background()

	t.Run("a system role cannot be shadowed, edited, or deleted", func(t *testing.T) {
		if _, err := s.Authz.CreateRole(ctx, s.Actor, domain.RoleOperations, "", nil); !errors.Is(err, identity.ErrSystemRole) {
			t.Errorf("CreateRole(operations) error = %v, want %v", err, identity.ErrSystemRole)
		}
		if err := s.Authz.UpdateRole(ctx, s.Actor, domain.RoleSuperAdmin, "", nil); !errors.Is(err, identity.ErrSystemRole) {
			t.Errorf("UpdateRole(super_admin) error = %v, want %v", err, identity.ErrSystemRole)
		}
		if err := s.Authz.DeleteRole(ctx, s.Actor, domain.RoleReadOnly); !errors.Is(err, identity.ErrSystemRole) {
			t.Errorf("DeleteRole(read_only) error = %v, want %v", err, identity.ErrSystemRole)
		}
	})

	// A misspelled permission must fail the whole role: silently granting the subset
	// would produce a role that looks right and authorizes less than it claims.
	t.Run("an unknown permission is refused outright", func(t *testing.T) {
		if _, err := s.Authz.CreateRole(ctx, s.Actor, "typo", "", []string{"transactions:read", "transaction:reed"}); err == nil {
			t.Error("CreateRole() accepted an unknown permission")
		}
		if exists := testsupport.Must(s.Authz.RoleExists(ctx, "typo")); exists {
			t.Error("the rejected role was persisted anyway")
		}
	})

	// An app-audience code must not be grantable to an administrator, or the two
	// namespaces would collapse into one.
	t.Run("an app permission is not an admin permission", func(t *testing.T) {
		if _, err := s.Authz.CreateRole(ctx, s.Actor, "wrongaudience", "", []string{"collections:create"}); err == nil {
			t.Error("CreateRole() granted an app-audience permission to a role")
		}
	})

	t.Run("a role still held cannot be deleted", func(t *testing.T) {
		testsupport.Must(s.Authz.CreateRole(ctx, s.Actor, "held", "", []string{"apps:read"}))
		testsupport.NoError(s.DB.Create(&domain.AdminUser{
			BaseModel: domain.BaseModel{ID: "admin-held"},
			Name:      "Holder",
			Email:     "holder@example.com",
			Role:      "held",
			Status:    "active",
		}).Error)
		if err := s.Authz.DeleteRole(ctx, s.Actor, "held"); err == nil {
			t.Error("DeleteRole() removed a role an administrator still holds")
		}
	})
}

func TestValidateAppScopes(t *testing.T) {
	for _, scopes := range []string{"transactions:read", "*", " COLLECTIONS:CREATE  transactions:read "} {
		if _, err := identity.ValidateAppScopes(scopes); err != nil {
			t.Errorf("identity.ValidateAppScopes(%q) error = %v", scopes, err)
		}
	}
	if got := testsupport.Must(identity.ValidateAppScopes("transactions:read transactions:read")); got != "transactions:read" {
		t.Errorf("identity.ValidateAppScopes() = %q, want duplicates collapsed", got)
	}
	for name, scopes := range map[string]string{
		"empty":        "   ",
		"typo":         "transaction:reed",
		"admin scope":  "users:create",
		"partly wrong": "transactions:read users:create",
	} {
		if _, err := identity.ValidateAppScopes(scopes); err == nil {
			t.Errorf("identity.ValidateAppScopes(%s) = nil, want an error", name)
		}
	}
}

func TestListPermissionsFiltersByAudience(t *testing.T) {
	s := testsupport.New(t)
	app := testsupport.Must(s.Authz.ListPermissions(context.Background(), domain.AudienceApp))
	for _, permission := range app {
		if permission.Audience != domain.AudienceApp {
			t.Errorf("audience filter returned %+v", permission)
		}
	}
	if _, err := s.Authz.ListPermissions(context.Background(), "nobody"); err == nil {
		t.Error("ListPermissions(nobody) = nil, want an error")
	}
}

func TestChangeRole(t *testing.T) {
	s := testsupport.New(t)
	ctx := context.Background()
	users := identity.NewAdminUserService(s.DB, audit.New(s.DB, nil), s.Authz)

	target := testsupport.Must(users.Create(ctx, s.Actor, "Target", "target@example.com", "password123", domain.RoleReadOnly))

	t.Run("reassigns to any existing role, seeded or custom", func(t *testing.T) {
		testsupport.Must(s.Authz.CreateRole(ctx, s.Actor, "support", "", []string{"transactions:read"}))
		testsupport.NoError(users.ChangeRole(ctx, s.Actor, target.ID, "support"))

		var stored domain.AdminUser
		testsupport.NoError(s.DB.First(&stored, "id = ?", target.ID).Error)
		if stored.Role != "support" {
			t.Fatalf("role = %q, want support", stored.Role)
		}
		// The point of resolving permissions per request: the new role takes effect
		// without revoking anything, so no session bookkeeping is involved.
		if permissions := testsupport.Must(s.Authz.EffectivePermissions(ctx, stored.Role)); len(permissions) != 1 {
			t.Errorf("effective permissions = %v, want the new role's single permission", permissions)
		}
	})

	// Both a lockout risk — the last super_admin demoting itself leaves nobody able to
	// undo it — and a privilege escalation, since users:update would otherwise be enough
	// to promote yourself.
	t.Run("refuses a self change", func(t *testing.T) {
		if err := users.ChangeRole(ctx, s.Actor, s.Actor.ID, domain.RoleReadOnly); err == nil {
			t.Error("ChangeRole() let an administrator change their own role")
		}
	})

	t.Run("refuses an unknown role and an unknown user", func(t *testing.T) {
		if err := users.ChangeRole(ctx, s.Actor, target.ID, "not_a_role"); err == nil {
			t.Error("ChangeRole() accepted an unknown role")
		}
		if err := users.ChangeRole(ctx, s.Actor, "missing", domain.RoleReadOnly); err == nil {
			t.Error("ChangeRole() accepted an unknown administrator")
		}
	})
}
