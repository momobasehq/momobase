package main

import (
	"strings"
	"testing"
)

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
