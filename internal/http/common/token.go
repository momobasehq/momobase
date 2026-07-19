package common

import (
	"net/http"

	"github.com/momobasehq/momobase/internal/platform"
)

// Token wraps a form-encoded grant handler with shared validation and JSON response handling.
func Token(grant string, issue func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			platform.Error(w, 400, "BAD_REQUEST", err.Error())
			return
		}
		actual := r.Form.Get("grant_type")
		if actual != grant && (grant != "password" || actual != "") {
			platform.Error(w, 400, "UNSUPPORTED_GRANT", "grant_type must be "+grant)
			return
		}
		out, err := issue(r)
		if err != nil {
			platform.Error(w, 401, "UNAUTHORIZED", err.Error())
			return
		}
		platform.RawJSON(w, 200, out)
	}
}
