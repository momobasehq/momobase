package admin

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFSContainsAdminPanelAssets(t *testing.T) {
	if err := fstest.TestFS(FS(), "index.html", "app.js", "sdk.js"); err != nil {
		t.Fatalf("embedded admin filesystem: %v", err)
	}

	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	if !strings.Contains(string(index), "<title>Momobase Admin</title>") {
		t.Fatal("embedded index does not contain the admin panel title")
	}
}
