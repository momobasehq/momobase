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

func BearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}
func AdminUser(r *http.Request) *domain.AdminUser {
	value, _ := r.Context().Value(adminKey).(*domain.AdminUser)
	return value
}
func App(r *http.Request) *services.AppIdentity {
	value, _ := r.Context().Value(appKey).(*services.AppIdentity)
	return value
}
func authenticate[T any](key key, verify func(string) (*T, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity, err := verify(BearerToken(r))
			if err != nil {
				platform.Error(w, 401, "UNAUTHORIZED", err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), key, identity)))
		})
	}
}
func WithAdminBearer(auth *services.AdminAuthService) func(http.Handler) http.Handler {
	return authenticate(adminKey, auth.AuthenticateBearer)
}
func WithAppBearer(auth *services.AppAuthService) func(http.Handler) http.Handler {
	return authenticate(appKey, auth.AuthenticateBearer)
}
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := AdminUser(r)
			if user == nil || !allowed[user.Role] {
				platform.Error(w, 403, "FORBIDDEN", "permission denied")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func RequireAppScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identity := App(r)
			if identity == nil || !hasScope(identity.Credential.Scopes, scope) {
				platform.Error(w, 403, "FORBIDDEN", "app credential is missing required scope")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
func hasScope(scopes, required string) bool {
	for _, scope := range strings.Fields(scopes) {
		if scope == required || scope == "*" {
			return true
		}
	}
	return false
}
