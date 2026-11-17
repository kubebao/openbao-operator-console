package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func normalizedCluster(t *testing.T, raw map[string]any) shared.Cluster {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ProjectCluster(
		data,
		adapter.ProjectionOptions{
			Origin:      "one",
			CollectedAt: time.Now().UTC(),
			Contract:    adapter.OpenBaoV1Alpha1,
			Limits:      adapter.DefaultProjectionLimits(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProjectionDropsProviderMessagesAndUnsupportedFields(t *testing.T) {
	raw := testCluster()
	raw["status"] = map[string]any{
		"remoteReplicaObservation": map[string]any{"freshness": map[string]any{"highestAppliedIndex": "9007199254741007"}},
		"conditions": []any{
			map[string]any{"type": "Available", "status": "Unknown", "reason": "Waiting", "message": "token=do-not-store"},
		},
	}
	d := projectCluster(
		normalizedCluster(t, raw),
		config.Connection{ID: "one"},
		config.Namespace{Team: "Platform", Environment: "production"},
	)
	if d.Replica != nil || d.ReadyReplicas != nil {
		t.Fatal("unsupported or missing values were fabricated")
	}
	encoded, err := json.Marshal(d)
	if err != nil || strings.Contains(string(encoded), "do-not-store") {
		t.Fatal("provider message leaked")
	}
	if fleet.DeploymentID("one", "team", "bao", "original") == fleet.DeploymentID("one", "team", "bao", "replacement") {
		t.Fatal("replacement inherited identity")
	}
}

func TestReadyReplicaOmissionUsesOnlyApprovedCurrentAcknowledgement(t *testing.T) {
	for _, observed := range []int{0, 3, 4, 5} {
		raw := testCluster()
		raw["status"] = map[string]any{"observedGeneration": observed}
		d := projectCluster(normalizedCluster(t, raw), config.Connection{ID: "one"}, config.Namespace{})
		if observed == 4 {
			if d.ReadyReplicas == nil || *d.ReadyReplicas != 0 {
				t.Fatal("documented zero omission lost")
			}
		} else if d.ReadyReplicas != nil {
			t.Fatalf("acknowledgement %d fabricated zero", observed)
		}
	}
}

func TestProjectionKeepsResourceAcknowledgementSeparateFromConditionAcknowledgement(t *testing.T) {
	for _, observed := range []any{nil, "invalid", 0, 3, 4, 5} {
		raw := testCluster()
		status := map[string]any{"conditions": []any{map[string]any{
			"type": "Available", "status": "True", "observedGeneration": 4,
		}}}
		if observed != nil {
			status["observedGeneration"] = observed
		}
		raw["status"] = status
		d := projectCluster(normalizedCluster(t, raw), config.Connection{ID: "one"}, config.Namespace{})
		if value, ok := observed.(int); ok {
			if d.ObservedGeneration == nil || *d.ObservedGeneration != int64(value) {
				t.Fatalf("resource acknowledgement lost: %+v", d)
			}
		} else if d.ObservedGeneration != nil {
			t.Fatal("condition acknowledgement filled in missing resource acknowledgement")
		}
	}
}

func TestBackupProjectionPreservesUnknownAndValidatedFields(t *testing.T) {
	raw := testCluster()
	raw["spec"].(map[string]any)["backup"] = map[string]any{}
	raw["status"].(map[string]any)["backup"] = map[string]any{
		"lastBackupTime":      "yesterday",
		"lastBackupName":      "s3://user:secret@bucket/key?token=1",
		"latestSnapshot":      map[string]any{"digest": "md5:abc"},
		"consecutiveFailures": -1,
		"lastBackupSize":      -1,
	}
	d := projectCluster(normalizedCluster(t, raw), config.Connection{ID: "one"}, config.Namespace{})
	if d.Backup == nil || d.Backup.ConsecutiveFailures != nil || d.Backup.Key != nil || d.Backup.Digest != nil ||
		d.Backup.LastSuccessAt != nil {
		t.Fatalf("invalid backup values survived: %+v", d.Backup)
	}
	raw["status"].(map[string]any)["backup"] = map[string]any{}
	d = projectCluster(normalizedCluster(t, raw), config.Connection{ID: "one"}, config.Namespace{})
	if d.Backup.ConsecutiveFailures != nil {
		t.Fatal("omitted failure counter became zero")
	}
}
