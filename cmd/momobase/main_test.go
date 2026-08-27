package main

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// TestProviderFactoriesAreRegistered guards the binary against shipping with an
// empty registry, which momobase.New rejects at startup.
func TestProviderFactoriesAreRegistered(t *testing.T) {
	factories := providerFactories()
	if len(factories) == 0 {
		t.Fatal("providerFactories() is empty; every command would fail at startup")
	}
	for code, factory := range factories {
		if factory == nil {
			t.Errorf("provider %q has a nil factory", code)
		}
	}
	if _, ok := factories["dummy"]; !ok {
		t.Error("the dummy provider must stay registered so a fresh deployment can be exercised")
	}
}

// TestLoadInstanceBuildsAServableInstance exercises the path every command
// shares. It is the only test that proves the registered providers actually
// initialize; the command tests below never reach it.
func TestLoadInstanceBuildsAServableInstance(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_PATH", filepath.Join(t.TempDir(), "momobase.db"))
	t.Setenv("ADMIN_OAUTH_SECRET", strings.Repeat("a", 32))
	t.Setenv("APP_OAUTH_SECRET", strings.Repeat("b", 32))
	t.Setenv("WORKERS_ENABLED", "false")

	instance, err := loadInstance()
	if err != nil {
		t.Fatalf("loadInstance() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := instance.Close(); closeErr != nil {
			t.Errorf("Close() error = %v", closeErr)
		}
	})
	if instance.App() == nil {
		t.Error("App() = nil, want the configured Fiber application")
	}
}

func TestVersionCommandReportsBuildInformation(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), []string{"version"}, &out); err != nil {
		t.Fatalf("run(version) error = %v", err)
	}
	for _, want := range []string{"momobase " + version, "commit: " + commit, "built:  " + date} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("version output %q does not contain %q", out.String(), want)
		}
	}
}

func TestVersionFlagReportsBuildInformation(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), []string{"--version"}, &out); err != nil {
		t.Fatalf("run(--version) error = %v", err)
	}
	if !strings.Contains(out.String(), "momobase "+version) {
		t.Fatalf("version output = %q", out.String())
	}
}

func TestSeedAdminFlagsRequirePassword(t *testing.T) {
	_, err := parseSeedAdmin(nil, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--password is required") {
		t.Fatalf("parseSeedAdmin() error = %v, want missing-password error", err)
	}
}

func TestSeedAdminFlagsAcceptPassword(t *testing.T) {
	options, err := parseSeedAdmin([]string{"--password", "strong-password"}, io.Discard)
	if err != nil {
		t.Fatalf("parseSeedAdmin() error = %v", err)
	}
	if options.password != "strong-password" || options.email != "admin@momobase.local" || options.name != "Super Admin" {
		t.Fatalf("parseSeedAdmin() = %+v, want password and defaults", options)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run(context.Background(), []string{"unknown"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("run(unknown) error = %v", err)
	}
}
