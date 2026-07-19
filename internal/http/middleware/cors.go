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
			if origin != "" && (allowed[origin] || allowed["*"]) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().
					Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, Idempotency-Key, X-CSRF-Token, X-Webhook-Secret")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, HEAD, OPTIONS")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
