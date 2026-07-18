package admin

import (
	"embed"
	"io/fs"
)

//go:embed index.html app.js sdk.js
var assets embed.FS

// FS returns the embedded administration panel assets.
func FS() fs.FS {
	return assets
}
