package config

import (
	"errors"
	"testing"
)

const (
	keycloakIssuer = "https://keycloak.example/realms/platform"
	gitlabIssuer   = "https://gitlab.example"
)

func issuerFixture() Config {
	c := Config{
		MaxBackupAgeHours: 24,
		Connections: []Connection{
			{
				ID:         "cluster",
				Name:       "Cluster",
				Access:     Access{Mode: AccessInCluster},
				Namespaces: []Namespace{{Name: "team", Environment: "production"}},
			},
		},
		Users: []User{
			{Subject: "42", Name: "Administrator", Admin: true, Scopes: []Scope{}},
			{Subject: "7", Name: "Reader", Scopes: []Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}}},
		},
		Groups: []Group{{Name: "admins", Admin: true, Scopes: []Scope{}}},
	}
	c.SeedIdentity(Identity{Issuer: keycloakIssuer, ClientID: "console"})
	return c
}

func TestResolveMatchesBindingsOnlyForTheirIssuer(t *testing.T) {
	c := issuerFixture()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if user, allowed := c.Resolve(Principal{Issuer: keycloakIssuer, Subject: "42"}); !allowed || !user.Admin {
		t.Fatal("administrator was not resolved under its own issuer")
	}
	for _, principal := range []Principal{
		{Issuer: gitlabIssuer, Subject: "42"},
		{Issuer: gitlabIssuer, Subject: "7"},
		{Issuer: gitlabIssuer, Subject: "unrelated", Groups: []string{"admins"}},
		{Subject: "42"},
	} {
		if user, allowed := c.Resolve(principal); allowed || user.Admin || len(user.Scopes) != 0 {
			t.Fatalf("binding matched principal from issuer %q", principal.Issuer)
		}
	}
}

func TestReplaceIdentityKeepsBindingsOnlyForTheSameIssuer(t *testing.T) {
	c := issuerFixture()
	rotated := Identity{Issuer: keycloakIssuer, ClientID: "console", ClientSecret: "rotated"}
	if err := c.ReplaceIdentity(rotated, ""); err != nil || len(c.Users) != 2 || c.Identity.ClientSecret != "rotated" {
		t.Fatal("same-issuer change did not keep bindings")
	}
	if err := c.ReplaceIdentity(rotated, AccessRulesRemove); !errors.Is(err, ErrAccessRulesUnexpected) {
		t.Fatal("same-issuer change accepted a removal decision")
	}
	gitlab := Identity{Issuer: gitlabIssuer, ClientID: "console"}
	if err := c.ReplaceIdentity(gitlab, ""); !errors.Is(err, ErrAccessRulesRequired) || c.AccessIssuer != keycloakIssuer {
		t.Fatal("issuer change proceeded without a decision")
	}
	if err := c.ReplaceIdentity(gitlab, "keep"); err == nil || c.AccessIssuer != keycloakIssuer {
		t.Fatal("unknown decision accepted")
	}

	carried := issuerFixture()
	if err := carried.ReplaceIdentity(gitlab, AccessRulesCarryOver); err != nil || carried.Validate() != nil {
		t.Fatal("carry over failed")
	}
	if user, allowed := carried.Resolve(Principal{Issuer: gitlabIssuer, Subject: "7"}); !allowed || len(user.Scopes) != 1 {
		t.Fatal("carried-over binding does not apply to the new issuer")
	}
	if _, allowed := carried.Resolve(Principal{Issuer: keycloakIssuer, Subject: "7"}); allowed {
		t.Fatal("carried-over binding still applies to the previous issuer")
	}

	removed := issuerFixture()
	if err := removed.ReplaceIdentity(gitlab, AccessRulesRemove); err != nil || len(removed.Users) != 0 || len(removed.Groups) != 0 {
		t.Fatal("removal kept bindings")
	}
	if _, allowed := removed.Resolve(Principal{Issuer: gitlabIssuer, Subject: "42", Groups: []string{"admins"}}); allowed {
		t.Fatal("removed binding matched the new issuer")
	}
}

func TestValidateRequiresBindingsForTheActiveIssuer(t *testing.T) {
	c := issuerFixture()
	c.Identity = &Identity{Issuer: gitlabIssuer, ClientID: "console"}
	if err := c.Validate(); err == nil {
		t.Fatal("identity changed without binding its access rules")
	}
	c = issuerFixture()
	c.Identity = nil
	if err := c.Validate(); err == nil {
		t.Fatal("access rules kept an issuer without an identity provider")
	}
}
