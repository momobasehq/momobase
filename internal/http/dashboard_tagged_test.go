//go:build dashboard

package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouterServesTheEmbeddedDashboard exercises the real embedded bundle, which
// only a tagged build carries. It is the counterpart to the !dashboard test that
// pins the unmounted case.
func TestRouterServesTheEmbeddedDashboard(t *testing.T) {
	app := testRouterWith(true)

	res := send(t, app, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "<title>Momobase Dashboard</title>") {
		t.Fatalf("GET /dashboard/ = %d %q", res.Code, res.Body)
	}
	if cache := res.Header.Get("Cache-Control"); cache != "no-cache" {
		t.Errorf("shell Cache-Control = %q, want no-cache", cache)
	}

	res = send(t, app, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if res.Code != http.StatusFound || res.Header.Get("Location") != "/dashboard/" {
		t.Errorf("GET /dashboard = %d %q", res.Code, res.Header.Get("Location"))
	}
}

// TestRouterRedirectsTheRetiredPanel keeps bookmarks of the deleted /admin/ panel
// working rather than answering them with a bare 404.
func TestRouterRedirectsTheRetiredPanel(t *testing.T) {
	app := testRouterWith(true)
	for _, target := range []string{"/admin/", "/admin"} {
		res := send(t, app, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusMovedPermanently || res.Header.Get("Location") != "/dashboard/" {
			t.Errorf("GET %s = %d %q, want 301 to /dashboard/", target, res.Code, res.Header.Get("Location"))
		}
	}
}
