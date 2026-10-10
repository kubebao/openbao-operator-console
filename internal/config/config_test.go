package config

import "testing"

func TestEnrollmentRejectsAmbiguousIdentityAndScope(t *testing.T) {
	valid := func() Config {
		return Config{
			MaxBackupAgeHours: 24,
			Connections: []Connection{
				{
					ID:         "one",
					Name:       "One",
					Access:     Access{Mode: AccessInCluster},
					Namespaces: []Namespace{{Name: "team", Environment: "production"}},
				},
			},
		}
	}
	for _, change := range []func(*Config){
		func(c *Config) { c.Connections = append(c.Connections, c.Connections[0]) },
		func(c *Config) {
			c.Users = []User{{Subject: "user", Scopes: []Scope{{ConnectionID: "one", Namespaces: []string{"other"}}}}}
		},
		func(c *Config) {
			c.Relationships = []Relationship{{
				ID:     "route",
				Kind:   "replication",
				Source: ResourceRef{ConnectionID: "one", Namespace: "team", Name: "bao"},
			}}
		},
	} {
		c := valid()
		change(&c)
		if err := c.Validate(); err == nil {
			t.Fatal("ambiguous enrollment was accepted")
		}
	}
	if err := valid().Validate(); err != nil {
		t.Fatal(err)
	}
}
