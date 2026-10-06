package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadSetupTokenRequiresPrivateBoundedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup-token")
	if err := os.WriteFile(path, []byte("private-token\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readSetupToken(path); err != nil || string(data) != "private-token\n" {
		t.Fatal("private token file was not read")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSetupToken(path); err == nil {
		t.Fatal("public token file was accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 257)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSetupToken(path); err == nil {
		t.Fatal("oversized token file was accepted")
	}
	if _, err := readSetupToken(filepath.Dir(path)); err == nil {
		t.Fatal("directory token path was accepted")
	}
}

func clearBootstrapEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{"CONSOLE_OIDC_ISSUER", "CONSOLE_OIDC_CLIENT_ID", "CONSOLE_OIDC_CLIENT_SECRET", "CONSOLE_OIDC_CA_FILE", "CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS"} {
		t.Setenv(key, "")
	}
}

func TestBootstrapSeedPermitsBrowserSetupAndRejectsPartialIdentity(t *testing.T) {
	clearBootstrapEnvironment(t)
	seed, err := bootstrapSeed("")
	if err != nil || seed.Identity != nil || seed.MaxBackupAgeHours != 24 {
		t.Fatal("empty identity seed did not permit protected browser setup")
	}
	t.Setenv("CONSOLE_OIDC_ISSUER", "https://identity.example")
	if _, err := bootstrapSeed(""); err == nil {
		t.Fatal("partial identity environment silently entered browser setup")
	}
	t.Setenv("CONSOLE_OIDC_CLIENT_ID", "console")
	t.Setenv("CONSOLE_OIDC_CLIENT_SECRET", "seed-secret")
	t.Setenv("CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS", "first,second")
	seed, err = bootstrapSeed("")
	if err != nil || seed.Identity == nil || len(seed.Users) != 2 || !seed.Users[0].Admin || !seed.Users[1].Admin {
		t.Fatal("preconfigured first-start identity or administrators were lost")
	}
}

func TestBootstrapSeedRetainsCompleteYAMLIdentity(t *testing.T) {
	clearBootstrapEnvironment(t)
	path := filepath.Join(t.TempDir(), "seed.yaml")
	if err := os.WriteFile(path, []byte("maxBackupAgeHours: 24\nstartupGraceMinutes: 10\nidentity:\n  issuer: https://identity.example\n  clientId: console\n  clientSecret: seed-secret\n  groupsClaim: groups\nusers:\n  - subject: seeded-admin\n    admin: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	seed, err := bootstrapSeed(path)
	if err != nil || seed.Identity == nil || seed.Identity.ClientID != "console" || len(seed.Users) != 1 || !seed.Users[0].Admin {
		t.Fatal("complete YAML identity was overwritten by absent environment")
	}
}
