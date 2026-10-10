package settings

import (
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
