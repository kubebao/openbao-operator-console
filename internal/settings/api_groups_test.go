package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func openGroupSettings(t *testing.T, repository Repository, key []byte, initial config.Config) *Service {
	t.Helper()
	service, err := Open(t.Context(), repository, key, &initial)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestAPIGroupsSurviveRemovalRestartAndEveryEditPath(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{}
	key := bytes.Repeat([]byte{8}, 32)
	service := openGroupSettings(t, repository, key, seed())
	original, revision := service.Current()
	removed := clone(original)
	removed.Connections = nil
	removed.Users = []config.User{{Subject: "admin", Admin: true}}
	if _, err := service.Save(t.Context(), revision, removed, "admin", "Remove enrollment"); err != nil {
		t.Fatal(err)
	}
	service, err := Open(t.Context(), repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.retainedReads != 0 {
		t.Fatal("declared bindings unexpectedly consulted observation state")
	}
	native := clone(original)
	native.Connections[0].APIGroup = config.APIGroupKubeBao
	_, revision = service.Current()
	assertGroupRebind := func(err error) {
		t.Helper()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "API group cannot change") {
			t.Fatalf("historical API-group rebind was not rejected: %v", err)
		}
	}
	_, err = service.Save(t.Context(), revision, native, "admin", "Rebind enrollment")
	assertGroupRebind(err)
	_, err = service.ApplyEdit(t.Context(), Edit{
		Revision: revision, Configuration: native,
		Servers:     map[string]string{"cluster": native.Connections[0].Access.Credential.Server},
		Credentials: map[string]config.Credential{"cluster": *native.Connections[0].Access.Credential},
	}, "admin")
	assertGroupRebind(err)
	_, err = service.ResolveConnection(native.Connections[0], "", nil)
	assertGroupRebind(err)
	assertGroupRebind(service.ValidateConnectionAPIGroup(native.Connections[0]))
	if _, after := service.Current(); after != revision {
		t.Fatal("rejected rebind changed the stored revision")
	}
	if _, err := service.Save(t.Context(), revision, original, "admin", "Re-enroll same API"); err != nil {
		t.Fatal("same-group enrollment was rejected", err)
	}
	current, revision := service.Current()
	newConnection := native.Connections[0]
	newConnection.ID = "native"
	current.Connections = append(current.Connections, newConnection)
	if _, err := service.Save(t.Context(), revision, current, "admin", "Add native enrollment"); err != nil {
		t.Fatal("new identity could not select native API", err)
	}
	public, _ := service.Current()
	data, err := json.Marshal(public)
	if err != nil || bytes.Contains(data, []byte("connectionAPIGroups")) {
		t.Fatal("server-owned API-group bindings entered the public configuration")
	}
}

func TestLegacyAPIGroupsBackfillWithoutRewritingSettings(t *testing.T) {
	t.Parallel()
	repository := &memoryRepository{retainedIDs: []string{"previously-removed"}}
	key := bytes.Repeat([]byte{9}, 32)
	service := openGroupSettings(t, repository, key, seed())
	legacy, revision := service.Current()
	payload, err := service.seal(legacy, configurationData(revision))
	if err != nil {
		t.Fatal(err)
	}
	repository.record.Payload = payload
	changes := len(repository.changes)
	service, err = Open(t.Context(), repository, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if repository.retainedReads != 1 || len(repository.changes) != changes || !bytes.Equal(repository.record.Payload, payload) {
		t.Fatal("legacy load changed the settings revision, ciphertext, or audit")
	}
	if service.apiGroups["cluster"] != config.APIGroupOpenBao || service.apiGroups["previously-removed"] != config.APIGroupOpenBao {
		t.Fatal("legacy current or retained identity was not bound")
	}
	connection := legacy.Connections[0]
	connection.ID, connection.APIGroup = "previously-removed", config.APIGroupKubeBao
	if !errors.Is(service.ValidateConnectionAPIGroup(connection), ErrInvalid) {
		t.Fatal("pre-upgrade retained identity was rebound to native")
	}
	if _, err := service.Save(t.Context(), revision, legacy, "admin", "Save settings"); err != nil {
		t.Fatal(err)
	}
	var stored storedConfiguration
	if err := service.unseal(repository.record.Payload, configurationData(revision+1), &stored); err != nil ||
		stored.APIGroups["previously-removed"] != config.APIGroupOpenBao {
		t.Fatal("normal save did not persist backfilled tombstones")
	}
	if _, err := Open(t.Context(), repository, key, nil); err != nil || repository.retainedReads != 1 {
		t.Fatal("persisted bindings did not survive restart independently of retained observations")
	}
}

func TestLegacyAPIGroupsBackfillFailsClosed(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ids  []string
		err  error
	}{
		{name: "read failed", err: errors.New("fixture read failed")},
		{name: "invalid identity", ids: []string{"invalid identity"}},
		{name: "limit", ids: make([]string, maxConnectionAPIGroups+1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository := &memoryRepository{retainedIDs: tc.ids, retainedErr: tc.err}
			key := bytes.Repeat([]byte{10}, 32)
			service := openGroupSettings(t, repository, key, seed())
			legacy, revision := service.Current()
			payload, err := service.seal(legacy, configurationData(revision))
			if err != nil {
				t.Fatal(err)
			}
			repository.record.Payload = payload
			if _, err := Open(t.Context(), repository, key, nil); err == nil {
				t.Fatal("legacy enrollment loaded without complete bounded identity bindings")
			}
		})
	}
}

func TestDeclaredAPIGroupsRejectInvalidOrMissingBindings(t *testing.T) {
	t.Parallel()
	configuration := seed()
	for _, raw := range []string{
		`null`, `{}`, `[]`, `{"cluster":"other.org"}`, `{"cluster":""}`,
		`{"cluster":"kubebao.org"}`, `{"cluster":"openbao.org","invalid identity":"openbao.org"}`,
	} {
		if _, err := readAPIGroups(json.RawMessage(raw), configuration); err == nil {
			t.Fatalf("invalid binding accepted: %s", raw)
		}
	}
	groups := map[string]config.APIGroup{}
	for i := range maxConnectionAPIGroups + 1 {
		groups[fmt.Sprintf("connection-%d", i)] = config.APIGroupOpenBao
	}
	if _, err := bindAPIGroups(groups, config.Config{}); err == nil {
		t.Fatal("lifetime API-group binding bound was ignored")
	}
}

func TestAPIGroupsPersistAfterCommittedReplyLoss(t *testing.T) {
	t.Parallel()
	repository := &unreliableRepository{memoryRepository: &memoryRepository{}}
	key := bytes.Repeat([]byte{11}, 32)
	service := openGroupSettings(t, repository, key, seed())
	current, revision := service.Current()
	original := current.Connections[0]
	current.Connections = nil
	current.Users = []config.User{{Subject: "admin", Admin: true}}
	repository.lostReplies = 1
	if _, err := service.Save(context.Background(), revision, current, "admin", "Remove enrollment"); err != nil {
		t.Fatal(err)
	}
	original.APIGroup = config.APIGroupKubeBao
	if !errors.Is(service.ValidateConnectionAPIGroup(original), ErrInvalid) {
		t.Fatal("refresh after a committed reply loss forgot removed API identity")
	}
}
