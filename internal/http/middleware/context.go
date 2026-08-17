package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/momobasehq/momobase/internal/domain"
	"github.com/momobasehq/momobase/internal/platform"
	"github.com/momobasehq/momobase/internal/services"
)

type key uint8

const (
	adminKey key = iota
	appKey
)

// BearerToken returns the token from a Bearer Authorization header, or an empty
// string when the header does not use the Bearer scheme.
func BearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

// AdminUser returns the authenticated administrator stored in the request
// context, or nil when no administrator is present.
func AdminUser(r *http.Request) *domain.AdminUser {
	value, _ := r.Context().Value(adminKey).(*domain.AdminUser)
	return value
}

// App returns the authenticated application identity stored in the request
// context, or nil when no application identity is present.
func App(r *http.Request) *services.AppIdentity {
	value, _ := r.Context().Value(appKey).(*services.AppIdentity)
	return value
}
func authenticate[T any](key key, verify func(context.Context, string) (*T, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, err := verify(r.Context(), BearerToken(r))
			if err != nil {
				platform.Error(w, 401, "UNAUTHORIZED", err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key, identity)))
		})
	}
}

// WithAdminBearer authenticates an administrator bearer token and stores the
// resulting administrator in the request context.
func WithAdminBearer(auth *services.AdminAuthService) func(http.Handler) http.Handler {
	return authenticate(adminKey, auth.AuthenticateBearer)
}

// WithAppBearer authenticates an application bearer token and stores the
// resulting application identity in the request context.
func WithAppBearer(auth *services.AppAuthService) func(http.Handler) http.Handler {
	return authenticate(appKey, auth.AuthenticateBearer)
}

// RequirePermission allows requests whose authenticated administrator holds the
// named permission, through their role, and rejects all others with a forbidden
// response.
//
// The permission is resolved from the role on every request rather than read from the
// token, so removing it from a role takes effect on the next call. An administrator
// whose role no longer exists resolves to no permissions and is therefore refused,
// which is the safe direction to fail.
func RequirePermission(permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := AdminUser(r)
			if user == nil || !granted(user.Permissions, permission) {
				platform.Error(w, 403, "FORBIDDEN", "permission denied: "+permission)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireAppScope allows requests whose authenticated application credential
// has the required scope or the wildcard scope.
func RequireAppScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := App(r)
			if identity == nil || !granted(strings.Fields(identity.Credential.Scopes), scope) {
				platform.Error(w, 403, "FORBIDDEN", "app credential is missing required scope")
				return
			}
			next.ServeHTTP(w, r)
		})
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
