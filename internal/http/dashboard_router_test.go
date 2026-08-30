package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRouterServesTheEmbeddedDashboard exercises the real embedded bundle.
func TestRouterServesTheEmbeddedDashboard(t *testing.T) {
	app := testRouterWith(true, "/operations")

	res := send(t, app, httptest.NewRequest(http.MethodGet, "/operations/", nil))
	if res.Code != http.StatusOK || !strings.Contains(res.Body, "<title>Momobase Dashboard</title>") {
		t.Fatalf("GET /operations/ = %d %q", res.Code, res.Body)
	}
	if cache := res.Header.Get("Cache-Control"); cache != "no-cache" {
		t.Errorf("GET /operations/ Cache-Control = %q, want no-cache", cache)
	}

	res = send(t, app, httptest.NewRequest(http.MethodGet, "/operations", nil))
	if res.Code != http.StatusMovedPermanently || res.Header.Get("Location") != "/operations/" {
		t.Errorf("GET /operations = %d %q, want 301 to /operations/", res.Code, res.Header.Get("Location"))
	}
}

// TestRouterRedirectsTheRetiredPanel keeps bookmarks of the deleted /admin/ panel
// working rather than answering them with a bare 404.
func TestRouterRedirectsTheRetiredPanel(t *testing.T) {
	app := testRouterWith(true, "/operations")
	for _, target := range []string{"/admin/", "/admin"} {
		res := send(t, app, httptest.NewRequest(http.MethodGet, target, nil))
		if res.Code != http.StatusMovedPermanently || res.Header.Get("Location") != "/operations/" {
			t.Errorf("GET %s = %d %q, want 301 to /operations/", target, res.Code, res.Header.Get("Location"))
		}
	}
}
