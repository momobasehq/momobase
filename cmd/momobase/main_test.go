package main

import (
	"strings"
	"testing"
)

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
