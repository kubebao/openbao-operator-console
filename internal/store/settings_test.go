package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func TestPostgresConfigurationRestartAndConcurrentRevision(t *testing.T) {
	dsn := os.Getenv("CONSOLE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated console_test PostgreSQL")
	}
	endpoint, err := url.Parse(dsn)
	if err != nil || endpoint.Path != "/console_test" {
		t.Fatal("requires console_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	owner, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	schemaName := fmt.Sprintf("configuration_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schemaName}.Sanitize()
	if _, err := owner.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = owner.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE") }()
	query := endpoint.Query()
	query.Set("search_path", schemaName)
	endpoint.RawQuery = query.Encode()
	open := func() *Postgres {
		t.Helper()
		repository, err := Open(ctx, endpoint.String(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return repository
	}
	repository := open()
	key := bytes.Repeat([]byte{6}, 32)
	token := "e30." + base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"private-integration-token"}`),
	) + ".signature"
	seed := config.Config{
		MaxBackupAgeHours: 24,
		Users:             []config.User{{Subject: "admin", Admin: true}},
		Connections: []config.Connection{
			{
				ID:         "test",
				Name:       "Test",
				Namespaces: []config.Namespace{{Name: "team", Environment: "development"}},
				Renewal:    &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"},
				Access: config.Access{
					Mode:       config.AccessRemote,
					Credential: &config.Credential{Server: "https://api.example", Token: token},
				},
			},
		},
	}
	if _, err := settings.New(ctx, repository, key, seed); err != nil {
		t.Fatal(err)
	}
	record, err := repository.LoadSettings(ctx)
	if err != nil || bytes.Contains(record.Payload, []byte("private-integration-token")) {
		t.Fatal("plaintext configuration persisted")
	}
	repository.Close()
	repository = open()
	defer repository.Close()
	service, err := settings.New(ctx, repository, key, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := settings.New(ctx, repository, key, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	configuration, revision := service.Current()
	configuration.MaxBackupAgeHours = 12
	if _, err := service.Save(ctx, revision, configuration, "admin", "Updated policies"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Save(ctx, revision, configuration, "other", "Stale save"); !errors.Is(err, settings.ErrConflict) {
		t.Fatalf("concurrent revision = %v", err)
	}
	restarted, err := settings.New(ctx, repository, key, seed)
	if err != nil {
		t.Fatal(err)
	}
	got, revision := restarted.Current()
	if got.MaxBackupAgeHours != 12 || revision != 2 || got.Connections[0].Access.Credential.Token != token {
		t.Fatal("restart overwrote stored configuration")
	}
	history, err := restarted.History(ctx)
	if err != nil || len(history) != 2 || history[0].Actor != "admin" || history[0].Revision != 2 {
		t.Fatal("configuration audit not committed with settings")
	}
	if _, err := settings.New(ctx, repository, bytes.Repeat([]byte{9}, 32), seed); err == nil {
		t.Fatal("wrong key decrypted configuration")
	}
	target := restarted.CredentialTargets()[0]
	state := target.State
	now := time.Now().UTC()
	expires, next := now.Add(time.Hour), now.Add(20*time.Minute)
	state.Credential.Token = "e30." + base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"replacement"}`),
	) + ".signature"
	state.State, state.ExpiryVerified = settings.RenewalHealthy, true
	state.ExpiresAt, state.NextAttemptAt = &expires, &next
	state.LastAttemptAt, state.LastSuccessAt = &now, &now
	if err := restarted.UpdateCredential(ctx, target, state); err != nil {
		t.Fatal(err)
	}
	rows, err := repository.LoadCredentials(ctx)
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 || bytes.Contains(rows[0].Payload, []byte(state.Credential.Token)) {
		t.Fatal("credential storage was not encrypted and independently versioned")
	}
	if err := repository.SaveCredential(ctx, revision, rows[0]); !errors.Is(err, settings.ErrConflict) {
		t.Fatal("stale credential revision accepted")
	}
	if err := repository.SaveCredential(
		ctx,
		revision-1,
		settings.CredentialRecord{ConnectionID: "test", Revision: 3, Payload: rows[0].Payload},
	); !errors.Is(
		err,
		settings.ErrConflict,
	) {
		t.Fatal("stale enrollment revision accepted")
	}
	configuration, revision = restarted.Current()
	configuration.MaxBackupAgeHours = 6
	// Configuration and credential replacements must roll back together on conflict.
	if _, err := repository.SaveSettings(
		ctx,
		revision,
		record.Payload,
		"admin",
		"must roll back",
		rows,
		[]string{"test"},
	); !errors.Is(
		err,
		settings.ErrConflict,
	) {
		t.Fatal("credential conflict accepted during settings commit")
	}
	after, err := repository.LoadSettings(ctx)
	if err != nil || after.Revision != revision {
		t.Fatal("failed credential write committed configuration")
	}
	if _, err := restarted.Save(ctx, revision, configuration, "admin", "Policies updated"); err != nil {
		t.Fatal(err)
	}
	reopened, err := settings.Open(ctx, repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	latest := reopened.CredentialTargets()[0]
	if latest.ConfigurationRevision != 3 || latest.State.Credential.Token != state.Credential.Token ||
		!latest.State.NextAttemptAt.Equal(next) {
		t.Fatal("restart lost current credential or schedule")
	}
}
