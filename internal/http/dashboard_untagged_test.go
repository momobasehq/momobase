//go:build !dashboard

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRouterDoesNotServeDashboardWithoutAssets pins the degradation contract: the
// default build embeds no bundle, so setting DASHBOARD_ENABLED against it must leave
// the route unmounted rather than serve a shell whose scripts are not there.
//
// It is tagged !dashboard because a tagged build mounts the route by design, which
// is what build.yml asserts against the real image.
func TestRouterDoesNotServeDashboardWithoutAssets(t *testing.T) {
	app := testRouterWith(true)
	for _, target := range []string{"/dashboard/", "/dashboard"} {
		res := send(t, app, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want %d in a build without the dashboard tag", target, res.Code, http.StatusNotFound)
		}
	}
}
