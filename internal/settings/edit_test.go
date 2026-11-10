package settings

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestResolveAccessKeepsOrReplacesTheReaderCredential(t *testing.T) {
	stored := &config.Credential{Server: "https://api.example", Token: "stored"}
	replacement := &config.Credential{Server: "https://new.example", Token: "replacement"}
	remote := config.Connection{ID: "cluster", Access: config.Access{Mode: config.AccessRemote}}
	local := config.Connection{ID: "cluster", Access: config.Access{Mode: config.AccessInCluster}}
	for _, test := range []struct {
		name        string
		connection  config.Connection
		server      string
		replacement *config.Credential
		stored      *config.Credential
		invalid     bool
		token       string // the resulting credential's token; empty for no credential
	}{
		{name: "keep stored", connection: remote, server: stored.Server, stored: stored, token: "stored"},
		{name: "replace", connection: remote, server: replacement.Server, replacement: replacement, stored: stored, token: "replacement"},
		{name: "new remote connection", connection: remote, server: replacement.Server, replacement: replacement, token: "replacement"},
		{name: "remote without credential", connection: remote, server: stored.Server, invalid: true},
		{name: "changed server without credential", connection: remote, server: "https://other.example", stored: stored, invalid: true},
		{name: "local drops stored credential", connection: local, stored: stored},
		{name: "local with credential", connection: local, replacement: replacement, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveAccess(test.connection, test.server, test.replacement, test.stored)
			if test.invalid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error = %v, want ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			token := ""
			if got.Access.Credential != nil {
				token = got.Access.Credential.Token
			}
			if token != test.token {
				t.Fatalf("credential token = %q, want %q", token, test.token)
			}
		})
	}
}

func TestApplyEditKeepsLocalAccountsAndDropsRemovedScopes(t *testing.T) {
	ctx := context.Background()
	initial := config.Config{
		MaxBackupAgeHours: 24,
		Connections: []config.Connection{
			{
				ID:     "cluster",
				Name:   "Cluster",
				Access: config.Access{Mode: config.AccessInCluster},
				Namespaces: []config.Namespace{
					{Name: "team", Environment: "production"},
					{Name: "old", Environment: "production"},
				},
			},
		},
		Users: []config.User{},
		LocalAccounts: []config.LocalAccount{
			{ID: "admin-id", Username: "admin", Admin: true, Scopes: []config.Scope{}},
			{
				ID:       "reader-id",
				Username: "reader",
				Scopes:   []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team", "old"}}},
			},
		},
	}
	service, err := Open(ctx, &memoryRepository{}, bytes.Repeat([]byte{1}, 32), &initial)
	if err != nil {
		t.Fatal(err)
	}
	// The settings document carries no local accounts, as the web editor sends it.
	edited, revision := service.Current()
	edited.LocalAccounts = nil
	edited.Connections[0].Namespaces = edited.Connections[0].Namespaces[:1]
	edited.MaxBackupAgeHours = 48
	if _, err := service.ApplyEdit(ctx, Edit{Revision: revision, Configuration: edited}, "local:admin"); err != nil {
		t.Fatalf("a local-only console could not save settings: %v", err)
	}
	current, _ := service.Current()
	if len(current.LocalAccounts) != 2 || current.MaxBackupAgeHours != 48 {
		t.Fatalf("local accounts after edit: %+v", current.LocalAccounts)
	}
	reader := current.LocalAccounts[1]
	if len(reader.Scopes) != 1 || len(reader.Scopes[0].Namespaces) != 1 || reader.Scopes[0].Namespaces[0] != "team" {
		t.Fatalf("removed namespace stayed in a local scope: %+v", reader.Scopes)
	}
}
