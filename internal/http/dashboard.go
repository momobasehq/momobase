package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// indexFile is the application shell every dashboard visit resolves to. The
// dashboard routes on the URL hash, which browsers never send, so this is the only
// document the server ever serves for it — there is no SPA fallback to get wrong.
const indexFile = "index.html"

// dashboardHandler serves the embedded dashboard bundle out of an fs.FS.
type dashboardHandler struct {
	assets fs.FS
	etags  map[string]string
}

// newDashboardHandler indexes the bundle and precomputes an entity tag per file.
//
// embed.FS reports a zero ModTime, so http.ServeContent emits no Last-Modified and
// a conditional request has nothing to validate against — every asset would be
// re-downloaded in full on every load. Hashing the contents once at start-up gives
// each file a stable validator that survives restarts and is identical across
// replicas, which a build timestamp would not be.
func newDashboardHandler(assets fs.FS) *dashboardHandler {
	handler := &dashboardHandler{assets: assets, etags: make(map[string]string)}
	_ = fs.WalkDir(assets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		handler.etags[name] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	return handler
}

// ServeHTTP serves one embedded asset, or the application shell at the root.
func (h *dashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Cleaning an absolute path collapses any ".." before it is used as a key, so a
	// traversal attempt resolves to a name that simply is not in the bundle.
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = indexFile
	}
	etag, ok := h.etags[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(h.assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// The shell names the hashed assets, so a cached copy would go on pointing at a
	// bundle the next deploy replaced; it must revalidate every time. The assets
	// carry their content hash in the filename and can never change under it.
	if name == indexFile {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
	w.Header().Set("ETag", etag)
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}
