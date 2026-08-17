package domain

// Permission audiences. A permission is granted either to an administrator through a
// role, or to an application credential as a scope. The two are separate namespaces
// because an application must never be grantable an administrative permission, and a
// few codes — transactions:read — are meaningful to both.
const (
	// AudienceAdmin marks a permission granted to administrators through a role.
	AudienceAdmin = "admin"
	// AudienceApp marks a permission granted to application credentials as a scope.
	AudienceApp = "app"
)

// PermissionWildcard grants every permission in its audience, including ones added by
// later releases. It is what keeps super_admin correct as the catalogue grows instead
// of needing a migration for each new endpoint.
const PermissionWildcard = "*"

// The seeded system roles. They cannot be edited or deleted, which is what makes
// re-synchronising their permission sets on every boot safe.
const (
	// RoleSuperAdmin holds the wildcard and can reach every endpoint.
	RoleSuperAdmin = "super_admin"
	// RoleOperations can read everything and query provider balances.
	RoleOperations = "operations"
	// RoleReadOnly can read everything except provider balances.
	RoleReadOnly = "read_only"
)

// PermissionDefinition describes one entry in the seeded permission catalogue.
type PermissionDefinition struct {
	// Code is the permission a route requires, formatted resource:action.
	Code string
	// Description explains the permission in an operator-facing list.
	Description string
	// Audience is AudienceAdmin or AudienceApp.
	Audience string
}

// Permissions is the canonical catalogue, and the only place a permission is defined.
//
// It lives in code rather than in a migration because a permission only means anything
// if some endpoint requires it: the two have to change together, and a route referring
// to a code absent from this list is a compile-time-visible mistake rather than a
// silent 403. Boot upserts these rows, so adding a guarded endpoint needs no migration.
var Permissions = []PermissionDefinition{
	{"system:read", "Read service information, health, and worker state", AudienceAdmin},
	{"transactions:read", "List and inspect transactions", AudienceAdmin},
	{"audit:read", "Read the audit log", AudienceAdmin},
	{"users:read", "List administrators", AudienceAdmin},
	{"users:create", "Create administrators", AudienceAdmin},
	{"users:update", "Change an administrator's password or status", AudienceAdmin},
	{"roles:read", "List roles and the permission catalogue", AudienceAdmin},
	{"roles:create", "Create roles", AudienceAdmin},
	{"roles:update", "Change a role's permissions", AudienceAdmin},
	{"roles:delete", "Delete roles", AudienceAdmin},
	{"apps:read", "List and inspect applications", AudienceAdmin},
	{"apps:create", "Register applications", AudienceAdmin},
	{"apps:update", "Change an application's details or status", AudienceAdmin},
	{"apps:test", "Run test payments with an application's own credentials", AudienceAdmin},
	{"credentials:read", "List application credentials", AudienceAdmin},
	{"credentials:create", "Issue application credentials", AudienceAdmin},
	{"credentials:update", "Rotate or revoke application credentials", AudienceAdmin},
	{"providers:read", "List provider accounts, the registry, and provider health", AudienceAdmin},
	{"providers:create", "Register provider accounts", AudienceAdmin},
	{"providers:update", "Change provider configuration, countries, or activation", AudienceAdmin},
	{"providers:test", "Run a provider configuration health check", AudienceAdmin},
	{"balances:read", "Query provider balances", AudienceAdmin},
	{"routes:read", "List payment routes", AudienceAdmin},
	{"routes:create", "Create payment routes", AudienceAdmin},
	{"routes:update", "Change a payment route's priority or activation", AudienceAdmin},

	{"collections:create", "Create collection payments", AudienceApp},
	{"disbursements:create", "Create disbursement payments", AudienceApp},
	{"transactions:read", "Read the application's own transactions", AudienceApp},
}

// SystemRoleDefinition describes one seeded role.
type SystemRoleDefinition struct {
	// Name identifies the role and is what AdminUser.Role stores.
	Name string
	// Description explains the role in an operator-facing list.
	Description string
	// Permissions lists the codes the role grants, or the wildcard.
	Permissions func() []string
}

// SystemRoles are seeded on every boot and are read-only.
//
// Their sets reproduce the authorization the hardcoded role checks enforced: an
// operations administrator could read every admin endpoint and query balances but
// mutate nothing, and everything else was super_admin only. read_only is new — the
// dashboard offered it while the API rejected it — and is operations without balances,
// which are the one read that reaches a provider's API rather than the database.
var SystemRoles = []SystemRoleDefinition{
	{
		Name:        RoleSuperAdmin,
		Description: "Full access, including every permission added by later releases",
		Permissions: func() []string { return []string{PermissionWildcard} },
	},
	{
		Name:        RoleOperations,
		Description: "Read everything and query provider balances; no changes",
		Permissions: func() []string { return AdminReadPermissions() },
	},
	{
		Name:        RoleReadOnly,
		Description: "Read everything except provider balances",
		Permissions: func() []string {
			codes := make([]string, 0, len(Permissions))
			for _, code := range AdminReadPermissions() {
				if code != "balances:read" {
					codes = append(codes, code)
				}
			}
			return codes
		},
	},
}

// AdminReadPermissions returns every administrative read permission, which is the
// basis of both non-privileged system roles.
func AdminReadPermissions() []string {
	codes := make([]string, 0, len(Permissions))
	for _, permission := range Permissions {
		if permission.Audience == AudienceAdmin && isRead(permission.Code) {
			codes = append(codes, permission.Code)
		}
	}
	return codes
}

// isRead reports whether a permission code names a read action.
func isRead(code string) bool {
	const suffix = ":read"
	return len(code) > len(suffix) && code[len(code)-len(suffix):] == suffix
}

// AppPermissionCodes returns the permission codes an application credential may hold,
// which is what validates a requested scope at credential creation.
func AppPermissionCodes() []string {
	codes := make([]string, 0, len(Permissions))
	for _, permission := range Permissions {
		if permission.Audience == AudienceApp {
			codes = append(codes, permission.Code)
		}
	}
	return codes
}
