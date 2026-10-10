package store

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func readerToken(marker string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"`+marker+`"}`),
	) + ".signature"
}

var testKey = bytes.Repeat([]byte{6}, 32)

func renewableSeed() config.Config {
	return config.Config{
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
					Credential: &config.Credential{Server: "https://api.example", Token: readerToken("private-integration-token")},
				},
			},
		},
	}
}

func TestPostgresConfigurationSurvivesRestartAndRejectsStaleRevisions(t *testing.T) {
	ctx := context.Background()
	open := isolatedDatabase(t)
	repository := open()
	if _, err := settings.New(ctx, repository, testKey, renewableSeed()); err != nil {
		t.Fatal(err)
	}
	record, err := repository.LoadSettings(ctx)
	if err != nil || bytes.Contains(record.Payload, []byte("private-integration-token")) {
		t.Fatal("plaintext configuration persisted")
	}
	repository.Close()
	repository = open()
	defer repository.Close()
	service, err := settings.New(ctx, repository, testKey, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := settings.New(ctx, repository, testKey, config.Config{})
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
	restarted, err := settings.New(ctx, repository, testKey, renewableSeed())
	if err != nil {
		t.Fatal(err)
	}
	got, revision := restarted.Current()
	if got.MaxBackupAgeHours != 12 || revision != 2 ||
		got.Connections[0].Access.Credential.Token != readerToken("private-integration-token") {
		t.Fatal("restart overwrote stored configuration")
	}
	history, err := restarted.History(ctx)
	if err != nil || len(history) != 2 || history[0].Actor != "admin" || history[0].Revision != 2 {
		t.Fatal("configuration audit not committed with settings")
	}
	if _, err := settings.New(ctx, repository, bytes.Repeat([]byte{9}, 32), renewableSeed()); err == nil {
		t.Fatal("wrong key decrypted configuration")
	}
}

func TestPostgresCredentialRevisionsCommitWithSettings(t *testing.T) {
	ctx := context.Background()
	repository := isolatedDatabase(t)()
	defer repository.Close()
	service, err := settings.New(ctx, repository, testKey, renewableSeed())
	if err != nil {
		t.Fatal(err)
	}
	record, err := repository.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	target := service.CredentialTargets()[0]
	state := target.State
	now := time.Now().UTC()
	expires, next := now.Add(time.Hour), now.Add(20*time.Minute)
	state.Credential.Token = readerToken("replacement")
	state.State, state.ExpiryVerified = settings.RenewalHealthy, true
	state.ExpiresAt, state.NextAttemptAt = &expires, &next
	state.LastAttemptAt, state.LastSuccessAt = &now, &now
	if err := service.UpdateCredential(ctx, target, state); err != nil {
		t.Fatal(err)
	}
	rows, err := repository.LoadCredentials(ctx)
	if err != nil || len(rows) != 1 || rows[0].Revision != 2 || bytes.Contains(rows[0].Payload, []byte(state.Credential.Token)) {
		t.Fatal("credential storage was not encrypted and independently versioned")
	}
	configuration, revision := service.Current()
	t.Run("stale credential revision", func(t *testing.T) {
		if err := repository.SaveCredential(ctx, revision, rows[0]); !errors.Is(err, settings.ErrConflict) {
			t.Fatal("stale credential revision accepted")
		}
	})
	t.Run("stale enrollment revision", func(t *testing.T) {
		stale := settings.CredentialRecord{ConnectionID: "test", Revision: 3, Payload: rows[0].Payload}
		if err := repository.SaveCredential(ctx, revision-1, stale); !errors.Is(err, settings.ErrConflict) {
			t.Fatal("stale enrollment revision accepted")
		}
	})
	t.Run("rollback on credential conflict", func(t *testing.T) {
		_, err := repository.SaveSettings(ctx, revision, record.Payload, "admin", "must roll back", rows, []string{"test"})
		if !errors.Is(err, settings.ErrConflict) {
			t.Fatal("credential conflict accepted during settings commit")
		}
		after, err := repository.LoadSettings(ctx)
		if err != nil || after.Revision != revision {
			t.Fatal("failed credential write committed configuration")
		}
	})
	configuration.MaxBackupAgeHours = 6
	if _, err := service.Save(ctx, revision, configuration, "admin", "Policies updated"); err != nil {
		t.Fatal(err)
	}
	reopened, err := settings.Open(ctx, repository, testKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	latest := reopened.CredentialTargets()[0]
	if latest.ConfigurationRevision != revision+1 || latest.State.Credential.Token != state.Credential.Token ||
		!latest.State.NextAttemptAt.Equal(next) {
		t.Fatal("restart lost current credential or schedule")
	}
}
