package dashboard

import (
	"embed"
	"io/fs"
)

// The all: prefix is load-bearing. Without it the embed pattern skips files whose
// names begin with an underscore, which is exactly how Rollup names its shared
// chunks — the build would succeed and the served app would 404 at run time.
//
//go:embed all:dist
var assets embed.FS

// FS returns the embedded dashboard assets rooted at the build output directory,
// so a path matches the URL the built index.html references.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "dist")
	if err != nil {
		// dist is embedded above, so this is unreachable outside a broken build.
		panic("dashboard: " + err.Error())
	}
	return sub
}
