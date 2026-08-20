package main

import (
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
	cmd := newVersionCommand()
	var out strings.Builder
	cmd.SetOut(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, want := range []string{"momobase " + version, "commit: " + commit, "built:  " + date} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("version output %q does not contain %q", out.String(), want)
		}
	}
}

func TestRootCommandCarriesVersion(t *testing.T) {
	if got := newRootCommand().Version; got != version {
		t.Fatalf("root command Version = %q, want %q", got, version)
	}
}

func TestSeedAdminCommandPasswordFlagHasNoDefault(t *testing.T) {
	flag := newSeedAdminCommand().Flags().Lookup("password")
	if flag == nil {
		t.Fatal("seed-admin must accept a password flag")
	}
	if flag.DefValue != "" {
		t.Fatalf("password flag default = %q, want empty", flag.DefValue)
	}
}

func TestSeedAdminCommandRequiresPasswordFlag(t *testing.T) {
	cmd := newSeedAdminCommand()
	cmd.SilenceUsage = true

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "password" not set`) {
		t.Fatalf("Execute() error = %v, want missing-password error", err)
	}
}

func TestSeedAdminCommandAcceptsPasswordFlag(t *testing.T) {
	cmd := newSeedAdminCommand()

	if err := cmd.ParseFlags([]string{"--password", "strong-password"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		t.Fatalf("GetString(password) error = %v", err)
	}
	if password != "strong-password" {
		t.Fatalf("password flag = %q, want parsed value", password)
	}
}
