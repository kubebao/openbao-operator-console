package config

import "testing"

func localFixture() Config {
	c := issuerFixture()
	c.Connections = append(c.Connections, Connection{
		ID:         "other",
		Name:       "Other",
		Access:     Access{Mode: AccessInCluster},
		Namespaces: []Namespace{{Name: "edge", Environment: "production"}},
	})
	c.LocalAccounts = []LocalAccount{
		{ID: "acct-admin", Username: "root-admin", Name: "Break glass", Admin: true, Scopes: []Scope{}},
		{
			ID:       "acct-reader",
			Username: "reader",
			Scopes:   []Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}},
		},
	}
	return c
}

func TestResolveMatchesLocalAccountsOnlyThroughTheLocalIssuer(t *testing.T) {
	c := localFixture()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	admin, allowed := c.Resolve(Principal{Issuer: LocalIssuer, Subject: "acct-admin"})
	if !allowed || !admin.Admin || admin.Subject != "local:root-admin" || admin.Name != "Break glass" || len(admin.Scopes) != 2 {
		t.Fatalf("local administrator resolved as %+v", admin)
	}
	reader, allowed := c.Resolve(Principal{Issuer: LocalIssuer, Subject: "acct-reader"})
	if !allowed || reader.Admin || reader.Name != "reader" || len(reader.Scopes) != 1 || !reader.CanView("cluster", "team") {
		t.Fatalf("local reader resolved as %+v", reader)
	}
	for _, principal := range []Principal{
		{Issuer: LocalIssuer, Subject: "42"},                            // an OIDC subject
		{Issuer: LocalIssuer, Subject: "root-admin"},                    // a username, not an ID
		{Issuer: keycloakIssuer, Subject: "acct-admin"},                 // an account ID from OIDC
		{Issuer: LocalIssuer, Subject: "x", Groups: []string{"admins"}}, // groups never apply
	} {
		if user, allowed := c.Resolve(principal); allowed || user.Admin {
			t.Fatalf("principal %+v resolved", principal)
		}
	}
}

func TestResolveIntersectsLocalScopesWithEnrollment(t *testing.T) {
	c := localFixture()
	c.LocalAccounts[1].Scopes = append(c.LocalAccounts[1].Scopes, Scope{ConnectionID: "removed", Namespaces: []string{"gone"}})
	reader, allowed := c.Resolve(Principal{Issuer: LocalIssuer, Subject: "acct-reader"})
	if !allowed || len(reader.Scopes) != 1 || reader.CanView("removed", "gone") {
		t.Fatalf("unenrolled scope survived: %+v", reader.Scopes)
	}
}

func TestLocalAccountsSurviveAnIssuerChange(t *testing.T) {
	c := localFixture()
	if err := c.ReplaceIdentity(Identity{Issuer: gitlabIssuer, ClientID: "console"}, AccessRulesRemove); err != nil {
		t.Fatal(err)
	}
	if len(c.LocalAccounts) != 2 || len(c.Users) != 0 {
		t.Fatal("issuer change touched local accounts")
	}
}

func TestValidateLocalAccounts(t *testing.T) {
	if c := localFixture(); c.Validate() != nil {
		t.Fatal("the fixture is invalid")
	}
	for name, mutate := range map[string]func(*Config){
		"duplicate id":       func(c *Config) { c.LocalAccounts[1].ID = "acct-admin" },
		"duplicate username": func(c *Config) { c.LocalAccounts[1].Username = "root-admin" },
		"uppercase username": func(c *Config) { c.LocalAccounts[1].Username = "Reader" },
		"username with at":   func(c *Config) { c.LocalAccounts[1].Username = "reader@example" },
		"empty id":           func(c *Config) { c.LocalAccounts[1].ID = "" },
		"long name":          func(c *Config) { c.LocalAccounts[1].Name = string(make([]byte, 257)) },
		"unenrolled scope": func(c *Config) {
			c.LocalAccounts[1].Scopes = []Scope{{ConnectionID: "removed", Namespaces: []string{"gone"}}}
		},
		"reserved issuer": func(c *Config) { c.SeedIdentity(Identity{Issuer: LocalIssuer, ClientID: "console"}) },
	} {
		c := localFixture()
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestFindLocalAccountIgnoresCase(t *testing.T) {
	c := localFixture()
	if account, found := c.FindLocalAccount(" Root-Admin "); !found || account.ID != "acct-admin" {
		t.Fatal("username lookup did not fold case")
	}
	if _, found := c.FindLocalAccount("missing"); found {
		t.Fatal("unknown username was found")
	}
	if ids := c.LocalAccountIDs(); len(ids) != 2 || ids[0] != "acct-admin" {
		t.Fatalf("unexpected IDs %v", ids)
	}
	id, err := NewLocalAccountID()
	if err != nil || !identifier.MatchString(id) || len(id) != 22 {
		t.Fatalf("invalid generated ID %q", id)
	}
}
