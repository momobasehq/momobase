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
	router := testRouterWith(true)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dashboard/", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "<title>Momobase Dashboard</title>") {
		t.Fatalf("GET /dashboard/ = %d %q", recorder.Code, recorder.Body.String())
	}
	if cache := recorder.Header().Get("Cache-Control"); cache != "no-cache" {
		t.Errorf("shell Cache-Control = %q, want no-cache", cache)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if recorder.Code != http.StatusFound || recorder.Header().Get("Location") != "/dashboard/" {
		t.Errorf("GET /dashboard = %d %q", recorder.Code, recorder.Header().Get("Location"))
	}
}

// TestRouterRedirectsTheRetiredPanel keeps bookmarks of the deleted /admin/ panel
// working rather than answering them with a bare 404.
func TestRouterRedirectsTheRetiredPanel(t *testing.T) {
	router := testRouterWith(true)
	for _, target := range []string{"/admin/", "/admin"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		if recorder.Code != http.StatusMovedPermanently || recorder.Header().Get("Location") != "/dashboard/" {
			t.Errorf("GET %s = %d %q, want 301 to /dashboard/", target, recorder.Code, recorder.Header().Get("Location"))
		}
	}
}
