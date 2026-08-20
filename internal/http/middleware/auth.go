package middleware

import (
	"context"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/service/identity"
)

type key uint8

const (
	adminKey key = iota
	appKey
)

// BearerToken returns the token from a Bearer Authorization header, or an empty
// string when the header does not use the Bearer scheme.
func BearerToken(c fiber.Ctx) string {
	value := c.Get(fiber.HeaderAuthorization)
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

// AdminUser returns the authenticated administrator stored on the request, or nil
// when no administrator is present.
func AdminUser(c fiber.Ctx) *domain.AdminUser {
	value, _ := c.Locals(adminKey).(*domain.AdminUser)
	return value
}

// App returns the authenticated application identity stored on the request, or nil
// when no application identity is present.
func App(c fiber.Ctx) *identity.AppIdentity {
	value, _ := c.Locals(appKey).(*identity.AppIdentity)
	return value
}

// authenticate verifies a bearer token's signature and then resolves the identity
// behind it. The two steps are separate so the response can distinguish a token the
// caller can fix by refreshing from a session that has been revoked.
func authenticate[T any](
	k key,
	verify func(string) (*platform.TokenClaims, error),
	resolve func(context.Context, *platform.TokenClaims) (*T, error),
) fiber.Handler {
	return func(c fiber.Ctx) error {
		claims, err := verify(BearerToken(c))
		if err != nil {
			// The verifier's own message names the failed claim, which is more than a
			// caller needs and more than it should learn.
			return platform.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
		}
		identity, err := resolve(c.Context(), claims)
		if err != nil {
			return platform.Error(c, fiber.StatusUnauthorized, "UNAUTHORIZED", err.Error())
		}
		c.Locals(k, identity)
		return c.Next()
	}
}

// WithAdminBearer authenticates an administrator bearer token and stores the
// resulting administrator on the request.
func WithAdminBearer(auth *identity.AdminAuthService) fiber.Handler {
	return authenticate(adminKey, auth.Verify, auth.AuthenticateClaims)
}

// WithAppBearer authenticates an application bearer token and stores the resulting
// application identity on the request.
func WithAppBearer(auth *identity.AppAuthService) fiber.Handler {
	return authenticate(appKey, auth.Verify, auth.AuthenticateClaims)
}

// RequirePermission allows requests whose authenticated administrator holds the
// named permission, through their role, and rejects all others with a forbidden
// response.
//
// The permission is resolved from the role on every request rather than read from the
// token, so removing it from a role takes effect on the next call. An administrator
// whose role no longer exists resolves to no permissions and is therefore refused,
// which is the safe direction to fail.
func RequirePermission(permission string) fiber.Handler {
	return func(c fiber.Ctx) error {
		user := AdminUser(c)
		if user == nil || !granted(user.Permissions, permission) {
			return platform.Error(c, fiber.StatusForbidden, "FORBIDDEN", "permission denied: "+permission)
		}
		return c.Next()
	}
}

// RequireAppScope allows requests whose authenticated application credential has the
// required scope or the wildcard scope.
func RequireAppScope(scope string) fiber.Handler {
	return func(c fiber.Ctx) error {
		identity := App(c)
		if identity == nil || !granted(strings.Fields(identity.Credential.Scopes), scope) {
			return platform.Error(c, fiber.StatusForbidden, "FORBIDDEN", "app credential is missing required scope")
		}
		return c.Next()
	}
}

// granted reports whether held satisfies required, honouring the wildcard. Admin roles
// and app credentials share it so the two authorization paths cannot drift on what a
// wildcard means.
func granted(held []string, required string) bool {
	for _, permission := range held {
		if permission == required || permission == domain.PermissionWildcard {
			return true
		}
	}
	return false
}
