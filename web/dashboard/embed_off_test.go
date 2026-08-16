//go:build !dashboard

package dashboard

import "testing"

// TestUnavailableWithoutTheBuildTag pins the contract the router relies on: an
// untagged binary reports no dashboard and hands back no filesystem, so the route
// is never mounted over assets that are not there.
func TestUnavailableWithoutTheBuildTag(t *testing.T) {
	if Available() {
		t.Error("Available() = true without the dashboard build tag")
	}
	if FS() != nil {
		t.Error("FS() returned a filesystem without the dashboard build tag")
	}
}
