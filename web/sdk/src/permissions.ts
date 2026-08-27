/**
 * The permission codes this release of Momobase seeds.
 *
 * The server is the source of truth — `admin.authz.permissions()` returns the live
 * catalogue — but a client that gates its own UI needs the codes at the point where a
 * screen is written, not at run time. These constants exist so that a mistyped code is
 * a compile error rather than a permission that silently never matches.
 *
 * A Go test asserts this list matches the server's catalogue exactly, in both
 * directions, so the two cannot drift apart unnoticed.
 */
export const AdminPermissions = {
	systemRead: "system:read",
	transactionsRead: "transactions:read",
	auditRead: "audit:read",
	usersRead: "users:read",
	usersCreate: "users:create",
	usersUpdate: "users:update",
	rolesRead: "roles:read",
	rolesCreate: "roles:create",
	rolesUpdate: "roles:update",
	rolesDelete: "roles:delete",
	appsRead: "apps:read",
	appsCreate: "apps:create",
	appsUpdate: "apps:update",
	appsTest: "apps:test",
	credentialsRead: "credentials:read",
	credentialsCreate: "credentials:create",
	credentialsUpdate: "credentials:update",
	providersRead: "providers:read",
	providersCreate: "providers:create",
	providersUpdate: "providers:update",
	providersTest: "providers:test",
	balancesRead: "balances:read",
	routesRead: "routes:read",
	routesCreate: "routes:create",
	routesUpdate: "routes:update",
} as const;

/** The scopes an application credential may hold. */
export const AppScopes = {
	collectionsCreate: "collections:create",
	disbursementsCreate: "disbursements:create",
	transactionsRead: "transactions:read",
} as const;

/** Grants every permission in its audience, including ones a later release adds. A
 * client checking a permission must honour it rather than enumerate around it. */
export const PermissionWildcard = "*";

/** A permission an administrator's role may grant. */
export type AdminPermission =
	(typeof AdminPermissions)[keyof typeof AdminPermissions];
/** A scope an application credential may hold. */
export type AppScope = (typeof AppScopes)[keyof typeof AppScopes];

/**
 * Any permission code.
 *
 * The `string & {}` arm is deliberate: it keeps editor autocomplete listing the known
 * codes while still accepting one a newer server introduced, so an older client does
 * not fail to compile against a catalogue that has grown.
 */
export type PermissionCode = AdminPermission | AppScope | (string & {});

/** Reports whether held satisfies required, honouring the wildcard. This mirrors the
 * server's own check, so client-side gating and server-side enforcement agree. */
export function permitted(
	held: readonly string[] | undefined,
	required: PermissionCode,
) {
	if (!held) return false;
	return held.includes(required) || held.includes(PermissionWildcard);
}
