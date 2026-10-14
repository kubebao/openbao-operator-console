package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestReadSetupTokenRequiresPrivateBoundedRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup-token")
	if err := os.WriteFile(path, []byte("private-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := readSetupToken(path); err != nil || string(data) != "private-token\n" {
		t.Fatal("private token file was not read")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readSetupToken(path); err == nil {
		t.Fatal("public token file was accepted")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 257)), 0o600); err != nil {
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
	for _, key := range []string{
		"CONSOLE_OIDC_ISSUER",
		"CONSOLE_OIDC_CLIENT_ID",
		"CONSOLE_OIDC_CLIENT_SECRET",
		"CONSOLE_OIDC_CA_FILE",
		"CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS",
	} {
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
	if err := os.WriteFile(
		path,
		[]byte(`maxBackupAgeHours: 24
startupGraceMinutes: 10
identity:
  issuer: https://identity.example
  clientId: console
  clientSecret: seed-secret
  groupsClaim: groups
users:
  - subject: seeded-admin
    admin: true
`),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	seed, err := bootstrapSeed(path)
	if err != nil || seed.Identity == nil || seed.Identity.ClientID != "console" || len(seed.Users) != 1 || !seed.Users[0].Admin {
		t.Fatal("complete YAML identity was overwritten by absent environment")
	}
	if seed.AccessIssuer != "https://identity.example" {
		t.Fatal("seed access rules were not bound to the YAML issuer")
	}
}

func TestBootstrapSeedBindsAccessToTheBootstrapIssuer(t *testing.T) {
	clearBootstrapEnvironment(t)
	t.Setenv("CONSOLE_OIDC_ISSUER", "https://identity.example")
	t.Setenv("CONSOLE_OIDC_CLIENT_ID", "console")
	t.Setenv("CONSOLE_OIDC_CLIENT_SECRET", "seed-secret")
	t.Setenv("CONSOLE_BOOTSTRAP_ADMIN_SUBJECTS", "first")
	seed, err := bootstrapSeed("")
	if err != nil || seed.AccessIssuer != "https://identity.example" || seed.Validate() != nil {
		t.Fatal("seed access rules were not bound to the bootstrap issuer")
	}
}

func TestRecoveryAppliesTheIssuerChangeRules(t *testing.T) {
	const stored, recovery = "https://keycloak.example/realms/platform", "https://gitlab.example"
	current := config.Config{
		MaxBackupAgeHours: 24,
		Users:             []config.User{{Subject: "42", Name: "Reader", Scopes: []config.Scope{}}, {Subject: "admin", Admin: true}},
		Groups:            []config.Group{{Name: "admins", Admin: true, Scopes: []config.Scope{}}},
	}
	current.SeedIdentity(config.Identity{Issuer: stored, ClientID: "console"})

	same, summary, err := recoveredConfiguration(current, config.Identity{Issuer: stored, ClientID: "rotated"}, []string{"recovery"}, "")
	if err != nil || len(same.Users) != 3 || len(same.Groups) != 1 || same.AccessIssuer != stored ||
		summary != "Identity provider and administrators recovered" {
		t.Fatal("same-issuer recovery did not keep access rules")
	}
	if _, _, err := recoveredConfiguration(
		current,
		config.Identity{Issuer: stored},
		[]string{"recovery"},
		config.AccessRulesRemove,
	); err == nil {
		t.Fatal("same-issuer recovery accepted an access rules decision")
	}
	gitlab := config.Identity{Issuer: recovery, ClientID: "console"}
	if _, _, err := recoveredConfiguration(current, gitlab, []string{"recovery"}, ""); err == nil ||
		!strings.Contains(err.Error(), "--recover-access-rules") {
		t.Fatal("issuer change recovered without a decision")
	}
	carried, _, err := recoveredConfiguration(current, gitlab, []string{"recovery"}, config.AccessRulesCarryOver)
	if err != nil || carried.AccessIssuer != recovery || len(carried.Users) != 3 || len(carried.Groups) != 1 || carried.Validate() != nil {
		t.Fatal("carry-over recovery did not bind access rules to the recovery issuer")
	}
	removed, summary, err := recoveredConfiguration(current, gitlab, []string{"recovery"}, config.AccessRulesRemove)
	if err != nil || removed.AccessIssuer != recovery || len(removed.Groups) != 0 || len(removed.Users) != 1 ||
		removed.Users[0].Subject != "recovery" || !removed.Users[0].Admin || !strings.Contains(summary, "removed") {
		t.Fatal("removal recovery kept access rules or skipped the recovery administrators")
	}
	if _, allowed := removed.Resolve(config.Principal{Issuer: recovery, Subject: "42"}); allowed {
		t.Fatal("removed binding matched the recovery issuer")
	}
}
