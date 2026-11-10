package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

type settingsMemory struct {
	mu     sync.Mutex
	record settings.Record
}

func (r *settingsMemory) LoadSettings(context.Context) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record.Revision == 0 {
		return settings.Record{}, settings.ErrMissing
	}
	return r.record, nil
}

func (r *settingsMemory) SaveSettings(
	_ context.Context, revision int64, payload []byte, _, _ string, _ []settings.CredentialRecord, _ []string,
) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision {
		return settings.Record{}, settings.ErrConflict
	}
	r.record = settings.Record{Revision: revision + 1, Payload: payload}
	return r.record, nil
}

func (r *settingsMemory) LoadCredentials(context.Context) ([]settings.CredentialRecord, error) {
	return nil, nil
}

func (r *settingsMemory) SaveCredential(context.Context, int64, settings.CredentialRecord) error {
	return nil
}

func (r *settingsMemory) SettingsHistory(context.Context) ([]settings.Change, error) { return nil, nil }

type credentialMemory struct{ records map[string]localauth.Record }

func (r *credentialMemory) GetLocalCredential(_ context.Context, id string) (localauth.Record, error) {
	record, ok := r.records[id]
	if !ok {
		return localauth.Record{}, localauth.ErrNotFound
	}
	return record, nil
}

func (r *credentialMemory) CreateLocalCredential(_ context.Context, record localauth.Record) error {
	if _, exists := r.records[record.AccountID]; exists {
		return localauth.ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *credentialMemory) UpdateLocalCredential(_ context.Context, record localauth.Record) error {
	if r.records[record.AccountID].Revision != record.Revision-1 {
		return localauth.ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *credentialMemory) DeleteLocalCredentials(context.Context, []string) error { return nil }

func TestRecoverLocalAdministratorRestoresAccessWithoutTheProvider(t *testing.T) {
	ctx := context.Background()
	key := bytes.Repeat([]byte{4}, 32)
	seed := config.Config{
		MaxBackupAgeHours:   24,
		Users:               []config.User{{Subject: "oidc-admin", Admin: true, Scopes: []config.Scope{}}},
		LocalAccounts:       []config.LocalAccount{{ID: "viewer-id", Username: "viewer", Scopes: []config.Scope{}}},
		LocalSignInDisabled: true,
	}
	seed.SeedIdentity(config.Identity{Issuer: "https://idp.example", ClientID: "console"})
	service, err := settings.Open(ctx, &settingsMemory{}, key, &seed)
	if err != nil {
		t.Fatal(err)
	}
	store, _ := localauth.NewStore(&credentialMemory{records: map[string]localauth.Record{}}, key)
	hasher, _ := localauth.NewHasher()
	local := localauth.NewAuthenticator(store, hasher, localauth.NewThrottle())
	t.Setenv("CONSOLE_OIDC_REDIRECT_URL", "https://console.example:8443/auth/callback")
	var out strings.Builder
	if err := recoverLocalAdministrator(ctx, service, local, "viewer", &out); err != nil {
		t.Fatal(err)
	}
	current, _ := service.Current()
	account, _ := current.FindLocalAccount("viewer")
	if !account.Admin || current.LocalSignInDisabled || len(current.LocalAccounts) != 1 {
		t.Fatalf("recovered configuration: %+v, disabled %v", current.LocalAccounts, current.LocalSignInDisabled)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	link := lines[len(lines)-1]
	token, found := strings.CutPrefix(link, "https://console.example:8443/enroll#token=")
	if !found {
		t.Fatalf("unexpected output %q", out.String())
	}
	if _, err := local.OpenLink(ctx, token); err != nil {
		t.Fatalf("printed link did not open: %v", err)
	}
	out.Reset()
	if err := recoverLocalAdministrator(ctx, service, local, "breakglass", &out); err != nil {
		t.Fatal(err)
	}
	current, _ = service.Current()
	if created, found := current.FindLocalAccount("breakglass"); !found || !created.Admin {
		t.Fatal("recovery did not create a new administrator")
	}
	if err := recoverLocalAdministrator(ctx, service, local, "Bad Name", &out); err == nil {
		t.Fatal("an invalid username was accepted")
	}
	t.Setenv("CONSOLE_OIDC_REDIRECT_URL", "")
	if err := recoverLocalAdministrator(ctx, service, local, "viewer", &out); err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("recovery ran without a public address")
	}
}
