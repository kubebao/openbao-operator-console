package store

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func apiGroupEnrollment(id string, group config.APIGroup) config.Connection {
	return config.Connection{
		ID: id, APIGroup: group, Name: "API qualification",
		Access:     config.Access{Mode: config.AccessInCluster},
		Namespaces: []config.Namespace{{Name: "team", Environment: fleet.EnvironmentDevelopment}},
	}
}

func assertAPIGroupRebindRejected(t *testing.T, ctx context.Context, service *settings.Service, repository testRepository,
	connection config.Connection,
) {
	t.Helper()
	before, err := repository.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	configuration, revision := service.Current()
	configuration.Connections = []config.Connection{connection}
	if _, err := service.Save(ctx, revision, configuration, "admin", "Rejected cross-group edit"); !errors.Is(err, settings.ErrInvalid) {
		t.Fatalf("cross-group connection reuse: %v", err)
	}
	after, err := repository.LoadSettings(ctx)
	if err != nil || after.Revision != before.Revision || !bytes.Equal(after.Payload, before.Payload) {
		t.Fatal("rejected API-group change modified encrypted settings")
	}
	history, err := service.History(ctx)
	if err != nil || len(history) == 0 || history[0].Revision != before.Revision {
		t.Fatal("rejected API-group change modified the settings audit")
	}
}

func TestAPIGroupBindingSurvivesRemovalAndRestart(t *testing.T) {
	for _, test := range []struct {
		name         string
		group, other config.APIGroup
	}{
		{"omitted-legacy", "", config.APIGroupKubeBao},
		{"native", config.APIGroupKubeBao, config.APIGroupOpenBao},
	} {
		t.Run(test.name, func(t *testing.T) {
			storageBackends(t, func(t *testing.T) {
				ctx := t.Context()
				open := isolatedDatabase(t)
				repository := open()
				defer func() { repository.Close() }()
				seed := config.Config{
					MaxBackupAgeHours: 24, Users: []config.User{{Subject: "admin", Admin: true}},
					Connections: []config.Connection{apiGroupEnrollment("original", test.group)},
				}
				service, err := settings.Open(ctx, repository, testKey, &seed)
				if err != nil {
					t.Fatal(err)
				}
				assertAPIGroupRebindRejected(t, ctx, service, repository, apiGroupEnrollment("original", test.other))
				configuration, revision := service.Current()
				configuration.Connections = nil
				if _, err := service.Save(ctx, revision, configuration, "admin", "Remove enrollment"); err != nil {
					t.Fatal(err)
				}
				// No collector cleanup occurs between removal and attempted re-enrollment.
				assertAPIGroupRebindRejected(t, ctx, service, repository, apiGroupEnrollment("original", test.other))
				repository.Close()
				repository = open()
				service, err = settings.Open(ctx, repository, testKey, nil)
				if err != nil {
					t.Fatal(err)
				}
				assertAPIGroupRebindRejected(t, ctx, service, repository, apiGroupEnrollment("original", test.other))
				configuration, revision = service.Current()
				configuration.Connections = []config.Connection{apiGroupEnrollment("original", test.group)}
				if _, err := service.Save(ctx, revision, configuration, "admin", "Restore same API enrollment"); err != nil {
					t.Fatal("same-group re-enrollment was rejected", err)
				}
			})
		})
	}
}

// legacyConfigurationPayload models the flat encrypted settings written before
// API-group bindings existed. It contains no credentials or provider resources.
func legacyConfigurationPayload(t *testing.T, configuration config.Config) []byte {
	t.Helper()
	block, err := aes.NewCipher(testKey)
	if err != nil {
		t.Fatal(err)
	}
	encryption, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	plaintext, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, encryption.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	return encryption.Seal(nonce, nonce, plaintext, []byte("console-configuration-v2:1"))
}

func TestLegacyRetainedHistoryBindsRemovedConnectionBeforeEnrollment(t *testing.T) {
	storageBackends(t, func(t *testing.T) {
		ctx := t.Context()
		open := isolatedDatabase(t)
		repository := open()
		defer func() { repository.Close() }()
		legacy := config.Config{MaxBackupAgeHours: 24, Users: []config.User{{Subject: "admin", Admin: true}}}
		payload := legacyConfigurationPayload(t, legacy)
		if _, err := repository.SaveSettings(ctx, 0, payload, "bootstrap", "Legacy settings", nil, nil); err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Microsecond)
		id := fleet.DeploymentID("retired", "team", "bao", "same-uid")
		deployment := fleet.Deployment{
			ID: id, ConnectionID: "retired", Name: "bao", Namespace: "team", UID: "same-uid",
			Health: fleet.HealthHealthy, Issues: []fleet.Issue{},
			Evidence: fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, CollectedAt: now, LastAttemptAt: now},
		}
		if err := repository.Apply(ctx, Batch{
			Connection:  fleet.Connection{ID: "retired", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: now},
			Deployments: []fleet.Deployment{deployment},
		}); err != nil {
			t.Fatal(err)
		}
		service, err := settings.Open(ctx, repository, testKey, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertAPIGroupRebindRejected(t, ctx, service, repository, apiGroupEnrollment("retired", config.APIGroupKubeBao))
		stored, err := repository.LoadSettings(ctx)
		if err != nil || stored.Revision != 1 || !bytes.Equal(stored.Payload, payload) {
			t.Fatal("loading legacy bindings rewrote settings")
		}
		configuration, revision := service.Current()
		configuration.MaxBackupAgeHours = 12
		if _, err := service.Save(ctx, revision, configuration, "admin", "Save other settings"); err != nil {
			t.Fatal(err)
		}
		repository.Close()
		repository = open()
		service, err = settings.Open(ctx, repository, testKey, nil)
		if err != nil {
			t.Fatal(err)
		}
		assertAPIGroupRebindRejected(t, ctx, service, repository, apiGroupEnrollment("retired", config.APIGroupKubeBao))
		history, err := firstHistory(repository, ctx, id)
		if err != nil || len(history) != 1 || history[0].Deployment.ID != id {
			t.Fatal("legacy observation history changed during API adoption")
		}
		configuration, revision = service.Current()
		configuration.Connections = []config.Connection{apiGroupEnrollment("native", config.APIGroupKubeBao)}
		if _, err := service.Save(ctx, revision, configuration, "admin", "Enroll native API under new ID"); err != nil {
			t.Fatal(err)
		}
		current, _ := service.Current()
		if len(current.Connections) != 1 || current.Connections[0].EffectiveAPIGroup() != config.APIGroupKubeBao {
			t.Fatal("native enrollment did not persist")
		}
	})
}
