package utils_test

import (
	"errors"
	"testing"

	"github.com/momobasehq/momobase/internal/utils"
)

func TestConfigHelpers(t *testing.T) {
	config := map[string]any{
		"name":    "  Acme  ",
		"enabled": "TRUE",
		"one":     1,
		"count":   "42",
		"nested": map[string]any{
			"value": " result ",
		},
	}
	if utils.String(config, "name") != "Acme" || !utils.Bool(config, "enabled") || !utils.Bool(config, "one") {
		t.Fatalf("string/bool helpers returned unexpected values")
	}
	if utils.Bool(config, "missing") || utils.Int(config, "count") != 42 || utils.Int(config, "bad") != 0 {
		t.Fatalf("bool/int helpers returned unexpected values")
	}
	if got := utils.Path(config, "nested.value"); got != "result" {
		t.Fatalf("Path() = %q", got)
	}
	if got := utils.Path(config, "nested.value.missing"); got != "" {
		t.Fatalf("Path(non-object) = %q", got)
	}
	if utils.First(" ", " first ", "second") != "first" || utils.First("", " ") != "" {
		t.Fatal("First() returned an unexpected value")
	}
	if utils.Slash("path") != "/path" || utils.Slash("/path") != "/path" {
		t.Fatal("Slash() returned an unexpected value")
	}
	primary := errors.New("primary")
	fallback := errors.New("fallback")
	if utils.FirstError(primary, fallback) != primary || utils.FirstError(nil, fallback) != fallback {
		t.Fatal("FirstError() returned an unexpected error")
	}
}
