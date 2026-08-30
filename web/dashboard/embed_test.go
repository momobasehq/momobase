package dashboard

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// assetReference matches the URLs Vite writes into the built index.html.
var assetReference = regexp.MustCompile(`\./assets/[A-Za-z0-9._@-]+`)

func indexHTML(t *testing.T) string {
	t.Helper()
	if err := fstest.TestFS(FS(), "index.html"); err != nil {
		t.Fatalf("embedded dashboard filesystem: %v", err)
	}
	index, err := fs.ReadFile(FS(), "index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	return string(index)
}

// TestEveryReferencedAssetIsEmbedded catches a bundle that shipped incomplete.
//
// Vite writes hashed asset names into index.html, and `all:` is what keeps the
// underscore-prefixed chunks among them from being skipped. Without this test a
// dropped chunk produces a served page whose script tags 404 — a white screen with
// no build error anywhere.
func TestEveryReferencedAssetIsEmbedded(t *testing.T) {
	references := assetReference.FindAllString(indexHTML(t), -1)
	if len(references) == 0 {
		t.Fatal("built index.html references no assets, so the bundle is not what was embedded")
	}
	for _, reference := range references {
		name := strings.TrimPrefix(reference, "./")
		if _, err := fs.Stat(FS(), name); err != nil {
			t.Errorf("index.html references %s, which is not embedded: %v", reference, err)
		}
	}
}
