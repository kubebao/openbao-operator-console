package settings

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func renewalToken(marker string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"system:serviceaccount:console-system:reader","marker":"`+marker+`"}`),
	) + ".signature"
}

func renewableSeed() config.Config {
	configuration := seed()
	configuration.Connections[0].Access.Credential.Token = renewalToken("initial")
	configuration.Connections[0].Renewal = &config.TokenRenewal{Namespace: "console-system", ServiceAccount: "reader"}
	return configuration
}

func renewedState(target RenewalTarget) CredentialState {
	now := time.Now().UTC()
	expires, next := now.Add(24*time.Hour), now.Add(8*time.Hour)
	state := target.State
	state.Credential.Token = renewalToken("renewed")
	state.State = RenewalHealthy
	state.ExpiresAt, state.NextAttemptAt = &expires, &next
	state.ExpiryVerified = true
	state.LastAttemptAt, state.LastSuccessAt = &now, &now
	return state
}

func TestRenewalSurvivesRestartAndStaleSettingsWithoutChangingRevision(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{4}, 32)
	service, err := New(ctx, repository, key, renewableSeed())
	if err != nil {
		t.Fatal(err)
	}
	stale, revision := service.Current()
	_, _, generation := service.ReaderConfiguration()
	target := service.CredentialTargets()[0]
	state := renewedState(target)
	if err := service.UpdateCredential(ctx, target, state); err != nil {
		t.Fatal(err)
	}
	current, currentRevision := service.Current()
	if currentRevision != revision || current.Connections[0].Access.Credential.Token != state.Credential.Token ||
		len(repository.changes) != 1 {
		t.Fatal("renewal changed editable settings or configuration history")
	}
	if err := service.ApplyCollection(
		ctx,
		revision,
		generation,
		func(context.Context) error { t.Fatal("old reader wrote evidence"); return nil },
	); !errors.Is(
		err,
		ErrConflict,
	) {
		t.Fatal("old collection generation remained active")
	}
	// A failure-status update leaves both collection and settings generations stable.
	_, _, currentGeneration := service.ReaderConfiguration()
	target = service.CredentialTargets()[0]
	state.State = RenewalPermissionDenied
	state.Failures = 1
	if err := service.UpdateCredential(ctx, target, state); err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyCollection(ctx, revision, currentGeneration, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	stale.MaxBackupAgeHours = 12
	if _, err := service.Save(ctx, revision, stale, "admin", "Policies updated"); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(ctx, repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := restarted.CredentialTargets()[0]
	if got.ConfigurationRevision != revision+1 || got.State.Credential.Token != state.Credential.Token ||
		got.State.State != RenewalPermissionDenied ||
		!got.State.NextAttemptAt.Equal(*state.NextAttemptAt) {
		t.Fatal("restart or stale form discarded the current credential or its schedule")
	}
	storedConfig, err := restarted.decrypt(repository.record.Payload)
	if err != nil || storedConfig.Connections[0].Access.Credential != nil ||
		storedConfig.Connections[0].Access.Mode != config.AccessRemote {
		t.Fatal("reader credential remained embedded in editable settings")
	}
	row := repository.credentials["cluster"]
	if bytes.Contains(row.Payload, []byte(state.Credential.Token)) {
		t.Fatal("reader token stored in plaintext")
	}
	row.ConnectionID = "different-connection"
	if _, err := restarted.decryptCredential(row); err == nil {
		t.Fatal("reader ciphertext was not bound to its connection")
	}
}

func TestDelayedRenewalCannotUndoReplacementPauseRemovalOrPolicyChange(t *testing.T) {
	for _, change := range []string{"replace", "pause", "remove", "disable renewal"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			repository := &memoryRepository{}
			service, err := New(ctx, repository, bytes.Repeat([]byte{4}, 32), renewableSeed())
			if err != nil {
				t.Fatal(err)
			}
			target := service.CredentialTargets()[0]
			configuration, revision := service.Current()
			servers := map[string]string{"cluster": configuration.Connections[0].Access.Credential.Server}
			var credentials map[string]config.Credential
			switch change {
			case "replace":
				replacement := *configuration.Connections[0].Access.Credential
				replacement.Token = renewalToken("manual-replacement")
				credentials = map[string]config.Credential{"cluster": replacement}
			case "pause":
				configuration.Connections[0].Disabled = true
			case "remove":
				configuration.Connections = nil
				configuration.Users = configuration.Users[:1]
			case "disable renewal":
				configuration.Connections[0].Renewal = nil
			}
			edit := Edit{Revision: revision, Configuration: configuration, Servers: servers, Credentials: credentials}
			if _, err := service.ApplyEdit(ctx, edit, "admin"); err != nil {
				t.Fatal(err)
			}
			if err := service.UpdateCredential(ctx, target, renewedState(target)); !errors.Is(err, ErrConflict) {
				t.Fatal("delayed renewal undid an administrative change")
			}
			current, _ := service.Current()
			if change == "replace" && current.Connections[0].Access.Credential.Token != renewalToken("manual-replacement") {
				t.Fatal("replacement lost")
			}
			if change == "remove" && len(repository.credentials) != 0 {
				t.Fatal("removed credential retained")
			}
		})
	}
}

func TestStoredRemoteConnectionRequiresItsCredential(t *testing.T) {
	ctx := context.Background()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{5}, 32)
	if _, err := New(ctx, repository, key, renewableSeed()); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, repository, key, nil); err != nil {
		t.Fatal(err)
	}
	delete(repository.credentials, "cluster")
	if _, err := Open(ctx, repository, key, nil); err == nil {
		t.Fatal("missing remote credential fell back to local authentication")
	}
}

type failingCredentials struct{ *memoryRepository }

func (failingCredentials) SaveCredential(context.Context, int64, CredentialRecord) error {
	return errors.New("storage unavailable")
}

func TestFailedCredentialStorageAndInvalidStateKeepCurrentReader(t *testing.T) {
	ctx := context.Background()
	service, err := New(ctx, failingCredentials{&memoryRepository{}}, bytes.Repeat([]byte{4}, 32), renewableSeed())
	if err != nil {
		t.Fatal(err)
	}
	target := service.CredentialTargets()[0]
	state := renewedState(target)
	if err := service.UpdateCredential(ctx, target, state); err == nil {
		t.Fatal("failed persistence reported success")
	}
	if got := service.CredentialTargets()[0]; got.CredentialRevision != target.CredentialRevision ||
		got.State.Credential.Token != target.State.Credential.Token {
		t.Fatal("failed persistence replaced the reader")
	}
	state.State = "unbounded-provider-error"
	if err := service.UpdateCredential(ctx, target, state); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded renewal state accepted")
	}
}
