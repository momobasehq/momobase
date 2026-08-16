//go:build !dashboard

package dashboard

import "io/fs"

// This is the default build: no Vite output is embedded, and none is committed.
//
// //go:embed fails to compile when its pattern matches nothing, so embedding an
// untracked directory would make `go build ./...` on a clean checkout depend on
// someone having run pnpm first. Keeping the embed behind a tag means the Go
// toolchain alone builds and tests this repository, and the release, container,
// and web CI builds opt in with -tags dashboard after building the bundle.

// Available reports whether this binary carries the dashboard assets.
func Available() bool { return false }

// FS returns nil, because this build embeds no dashboard assets. Callers must
// check Available first; the router declines to mount the route without it, so a
// binary built without the tag serves nothing rather than an empty shell.
func FS() fs.FS { return nil }
