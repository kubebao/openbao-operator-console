package settings

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func localService(t *testing.T, users []config.User) *Service {
	t.Helper()
	initial := config.Config{
		MaxBackupAgeHours: 24,
		Connections: []config.Connection{{
			ID: "cluster", Name: "Cluster", Access: config.Access{Mode: config.AccessInCluster},
			Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
		}},
		Users:         users,
		LocalAccounts: []config.LocalAccount{{ID: "admin-id", Username: "admin", Admin: true, Scopes: []config.Scope{}}},
	}
	if len(users) > 0 {
		initial.SeedIdentity(config.Identity{Issuer: "https://idp.example", ClientID: "console"})
	}
	service, err := Open(context.Background(), &memoryRepository{}, bytes.Repeat([]byte{1}, 32), &initial)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestLocalAccountChangesUseRevisionsAndKeepAnAdministrator(t *testing.T) {
	service := localService(t, nil)
	ctx := context.Background()
	_, revision := service.Current()
	reader := config.LocalAccount{
		ID:       "reader-id",
		Username: "reader",
		Scopes:   []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}},
	}
	revision, err := service.AddLocalAccount(ctx, revision, reader, "local:admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AddLocalAccount(ctx, revision-1, reader, "local:admin"); !errors.Is(err, ErrConflict) {
		t.Fatal("a stale revision was accepted")
	}
	if _, err := service.AddLocalAccount(
		ctx,
		revision,
		config.LocalAccount{ID: "other", Username: "reader", Scopes: []config.Scope{}},
		"x",
	); !errors.Is(
		err,
		ErrInvalid,
	) {
		t.Fatal("a duplicate username was accepted")
	}
	reader.Name, reader.Admin, reader.Username = "Reader", true, "renamed"
	revision, err = service.UpdateLocalAccount(ctx, revision, reader, "local:admin")
	if err != nil {
		t.Fatal(err)
	}
	current, _ := service.Current()
	if account, _ := current.FindLocalAccount("reader"); !account.Admin || account.Name != "Reader" {
		t.Fatalf("update did not apply or renamed the account: %+v", current.LocalAccounts)
	}
	revision, err = service.RemoveLocalAccount(ctx, revision, "admin-id", "local:reader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RemoveLocalAccount(ctx, revision, "reader-id", "local:reader"); !errors.Is(err, ErrInvalid) {
		t.Fatal("the last administrator was removed")
	}
	if _, err := service.SetLocalSignIn(ctx, revision, false, "local:reader"); !errors.Is(err, ErrInvalid) {
		t.Fatal("local sign-in was disabled without an identity provider")
	}
	history, _ := service.History(ctx)
	if last := history[len(history)-1]; last.Summary != "Removed local account admin" || last.Actor != "local:reader" {
		t.Fatalf("change log: %+v", last)
	}
}

func TestDisablingLocalSignInRequiresAnOIDCAdministrator(t *testing.T) {
	service := localService(t, []config.User{{Subject: "viewer", Name: "Viewer", Scopes: []config.Scope{}}})
	ctx := context.Background()
	_, revision := service.Current()
	if _, err := service.SetLocalSignIn(ctx, revision, false, "local:admin"); !errors.Is(err, ErrInvalid) {
		t.Fatal("local sign-in was disabled while only local administrators existed")
	}
	service = localService(t, []config.User{{Subject: "oidc-admin", Name: "Admin", Admin: true, Scopes: []config.Scope{}}})
	_, revision = service.Current()
	if _, err := service.SetLocalSignIn(ctx, revision, false, "local:admin"); err != nil {
		t.Fatal(err)
	}
	current, _ := service.Current()
	if !current.LocalSignInDisabled {
		t.Fatal("local sign-in stayed enabled")
	}
}
