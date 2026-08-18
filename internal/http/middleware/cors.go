package middleware

import "net/http"

// CORS returns a middleware that adds the appropriate CORS headers to the response for requests from allowed origins. If no origins are provided, it defaults to allowing requests from http://localhost:9090.
func CORS(origins []string) Middleware {
	allowed := map[string]bool{}
	if len(origins) == 0 {
		origins = []string{"http://localhost:9090"}
	}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			// Always advertised, even when the origin is refused: the response body
			// differs by Origin either way, and a shared cache that missed that could
			// serve one origin's permissive response to another.
			w.Header().Add("Vary", "Origin")
			if origin != "" && (allowed[origin] || allowed["*"]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().
					Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Idempotency-Key, X-CSRF-Token, X-Webhook-Secret")
				// Every method the router actually serves. A method missing here fails
				// preflight rather than the request, so the browser reports it as a CORS
				// error with no status to trace — DELETE was absent and role deletion
				// could not be called cross-origin at all.
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, HEAD, OPTIONS")
				// Without this the browser preflights every single request.
				w.Header().Set("Access-Control-Max-Age", "600")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			// A refused preflight must not fall through to the router, which has no
			// OPTIONS route and would answer 405 — an opaque failure for the caller.
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
