package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// bundle stands in for a real Vite build. Testing against a fixture rather than the
// embedded assets keeps these cases running in the default build, which carries no
// dashboard at all.
func bundle() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                 {Data: []byte(`<!doctype html><title>Momobase Dashboard</title><div id="root"></div>`)},
		"assets/index-abc123.js":     {Data: []byte("console.log('app')")},
		"assets/index-abc123.css":    {Data: []byte(".app{}")},
		"assets/_commonjsHelpers.js": {Data: []byte("export {}")},
		"logo.svg":                   {Data: []byte("<svg/>")},
	}
}

func dashboardRequest(t *testing.T, handler http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	http.StripPrefix("/dashboard/", handler).ServeHTTP(recorder, request)
	return recorder
}

func TestDashboardServesTheShellAndAssets(t *testing.T) {
	handler := newDashboardHandler(bundle())
	tests := []struct {
		name        string
		path        string
		body        string
		contentType string
		cache       string
	}{
		{"shell", "/dashboard/", "Momobase Dashboard", "text/html", "no-cache"},
		{"script", "/dashboard/assets/index-abc123.js", "console.log", "javascript", "immutable"},
		{"stylesheet", "/dashboard/assets/index-abc123.css", ".app{}", "css", "immutable"},
		// Rollup names its shared chunks with a leading underscore, which is why the
		// embed uses all:. Serving one proves the whole bundle is reachable.
		{"underscored chunk", "/dashboard/assets/_commonjsHelpers.js", "export {}", "javascript", "immutable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := dashboardRequest(t, handler, test.path, nil)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), test.body) {
				t.Fatalf("GET %s = %d %q", test.path, recorder.Code, recorder.Body.String())
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, test.contentType) {
				t.Errorf("GET %s Content-Type = %q, want %q", test.path, contentType, test.contentType)
			}
			if cache := recorder.Header().Get("Cache-Control"); !strings.Contains(cache, test.cache) {
				t.Errorf("GET %s Cache-Control = %q, want %q", test.path, cache, test.cache)
			}
			if recorder.Header().Get("ETag") == "" {
				t.Errorf("GET %s served no ETag", test.path)
			}
		})
	}
}

// TestDashboardRevalidatesWithETag is the point of precomputing them: embed.FS has
// a zero ModTime, so without an entity tag every asset is re-downloaded in full on
// every single load.
func TestDashboardRevalidatesWithETag(t *testing.T) {
	handler := newDashboardHandler(bundle())
	first := dashboardRequest(t, handler, "/dashboard/assets/index-abc123.js", nil)
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on the first response")
	}

	second := dashboardRequest(t, handler, "/dashboard/assets/index-abc123.js", map[string]string{"If-None-Match": etag})
	if second.Code != http.StatusNotModified {
		t.Fatalf("conditional GET = %d, want %d", second.Code, http.StatusNotModified)
	}
	if second.Body.Len() != 0 {
		t.Errorf("304 carried a %d-byte body", second.Body.Len())
	}
}

// TestDashboardMissingPathsAre404 covers what hash routing buys: because every route
// lives after the #, the server never has to guess whether an unknown path is a
// client route. A missing asset is simply missing, and can never be answered with
// HTML that a browser would try to execute as a script.
func TestDashboardMissingPathsAre404(t *testing.T) {
	handler := newDashboardHandler(bundle())
	for _, target := range []string{
		"/dashboard/assets/missing.js",
		"/dashboard/providers/pacc_1",
		"/dashboard/nope.html",
		"/dashboard/../go.mod",
	} {
		recorder := dashboardRequest(t, handler, target, nil)
		if recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want %d", target, recorder.Code, http.StatusNotFound)
		}
	}
}
