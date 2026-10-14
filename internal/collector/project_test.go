package collector

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestProjectionPreservesExactIndexesAndDropsProviderMessages(t *testing.T) {
	cluster := unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{"version": "2.7.0", "replicas": int64(3)},
		"status": map[string]any{
			"remoteReplicaObservation": map[string]any{
				"freshness": map[string]any{"highestAppliedIndex": "9007199254741007"},
			},
			"conditions": []any{
				map[string]any{"type": "Available", "status": "Unknown", "reason": "Waiting", "message": "token=do-not-store"},
			},
		},
	}}
	cluster.SetName("bao")
	cluster.SetNamespace("team")
	cluster.SetUID(types.UID("original"))
	cluster.SetCreationTimestamp(metav1.NewTime(time.Now()))
	d := Project(cluster, config.Connection{ID: "one"}, config.Namespace{Team: "Platform", Environment: "production"}, time.Now())
	if d.Replica == nil || d.Replica.LocalIndex == nil || *d.Replica.LocalIndex != "9007199254741007" {
		t.Fatal("exact index was lost")
	}
	if d.ReadyReplicas != nil || d.Health != "unknown" {
		t.Fatal("missing readiness became zero or healthy")
	}
	if encoded, err := json.Marshal(d); err != nil || strings.Contains(string(encoded), "do-not-store") {
		t.Fatal("raw provider message leaked")
	}
	if fleet.DeploymentID("one", "team", "bao", "original") == fleet.DeploymentID("one", "team", "bao", "replacement") {
		t.Fatal("replacement inherited identity")
	}
}

// The operator API declares status.readyReplicas with omitempty, so zero ready replicas
// arrive as an absent field.
func TestOmittedReadyReplicasCountAsZeroOnceObserved(t *testing.T) {
	now := time.Now()
	cluster := func(status map[string]any) unstructured.Unstructured {
		result := unstructured.Unstructured{Object: map[string]any{
			"spec":   map[string]any{"replicas": int64(3)},
			"status": status,
		}}
		result.SetName("bao")
		result.SetNamespace("team")
		result.SetGeneration(4)
		return result
	}
	project := func(status map[string]any) fleet.Deployment {
		d := Project(cluster(status), config.Connection{ID: "one"}, config.Namespace{}, now)
		evidence.Evaluate(&d, now.Add(-time.Hour), false, fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: time.Hour}, now)
		return d
	}
	observed := project(map[string]any{"observedGeneration": int64(4)})
	if observed.ReadyReplicas == nil || *observed.ReadyReplicas != 0 || observed.Health != fleet.HealthCritical ||
		len(observed.Issues) != 1 || observed.Issues[0].Rule != fleet.RuleReplicaReadiness {
		t.Fatalf("observed status without ready replicas = %v, %s, %+v", observed.ReadyReplicas, observed.Health, observed.Issues)
	}
	for name, status := range map[string]map[string]any{
		"no status observation": {},
		"older generation":      {"observedGeneration": int64(3)},
		"invalid count":         {"observedGeneration": int64(4), "readyReplicas": "three"},
	} {
		if d := project(status); d.ReadyReplicas != nil || d.Health != fleet.HealthUnknown || len(d.Issues) != 0 {
			t.Fatalf("%s: readiness = %v, %s, %d findings, want unknown", name, d.ReadyReplicas, d.Health, len(d.Issues))
		}
	}
	if d := project(map[string]any{"observedGeneration": int64(4), "readyReplicas": int64(3)}); d.ReadyReplicas == nil ||
		*d.ReadyReplicas != 3 || d.Health != fleet.HealthHealthy {
		t.Fatalf("reported readiness = %v, %s", d.ReadyReplicas, d.Health)
	}
}

func TestBackupProjectionKeepsOnlyValidatedFields(t *testing.T) {
	if projectBackup(map[string]any{"spec": map[string]any{}}) != nil {
		t.Fatal("backup evidence was fabricated for a cluster without backup configuration")
	}
	digest := "sha256:" + strings.Repeat("a", 64)
	backup := projectBackup(map[string]any{
		"spec": map[string]any{"backup": map[string]any{"schedule": "@daily"}},
		"status": map[string]any{"backup": map[string]any{
			"lastBackupTime":      "2026-10-06T10:00:00Z",
			"nextScheduledBackup": "2026-10-07T10:00:00Z",
			"lastBackupName":      "backups/bao-20261006.snap",
			"latestSnapshot":      map[string]any{"digest": digest},
			"consecutiveFailures": int64(2),
			"lastBackupSize":      int64(4096),
		}},
	})
	if backup == nil || backup.LastSuccessAt == nil || !backup.LastSuccessAt.Equal(time.Date(2026, 10, 6, 10, 0, 0, 0, time.UTC)) ||
		backup.NextAt == nil || backup.Key == nil || *backup.Key != "backups/bao-20261006.snap" ||
		backup.Digest == nil || *backup.Digest != digest || backup.ConsecutiveFailures != 2 ||
		backup.SizeBytes == nil || *backup.SizeBytes != 4096 || backup.RestoreTest != nil {
		t.Fatalf("backup projection = %+v", backup)
	}
	invalid := projectBackup(map[string]any{
		"spec": map[string]any{"backup": map[string]any{}},
		"status": map[string]any{"backup": map[string]any{
			"lastBackupTime":      "yesterday",
			"lastBackupName":      "s3://user:secret@bucket/key?token=1",
			"latestSnapshot":      map[string]any{"digest": "md5:abc"},
			"consecutiveFailures": int64(-1),
			"lastBackupSize":      int64(-1),
		}},
	})
	if invalid == nil || invalid.LastSuccessAt != nil || invalid.Key != nil || invalid.Digest != nil ||
		invalid.ConsecutiveFailures != 0 || invalid.SizeBytes != nil {
		t.Fatalf("invalid backup status was retained: %+v", invalid)
	}
}

func TestRestoreTestProjectionSeparatesFailureFromBlockedAndRunning(t *testing.T) {
	passed := func(status, reason string) map[string]any {
		return map[string]any{"type": "Passed", "status": status, "reason": reason}
	}
	for name, tc := range map[string]struct {
		test    map[string]any
		outcome fleet.RestoreOutcome
		reason  string
	}{
		"never run": {test: map[string]any{}, outcome: fleet.RestoreOutcomeNotRun},
		"passed": {
			test:    map[string]any{"last": map[string]any{"outcome": "Passed"}, "conditions": []any{passed("True", "Passed")}},
			outcome: fleet.RestoreOutcomePassed,
			reason:  "",
		},
		"failed": {
			test: map[string]any{
				"last":       map[string]any{"outcome": "Failed", "reason": "Mismatch"},
				"conditions": []any{passed("False", "Failed")},
			},
			outcome: fleet.RestoreOutcomeFailed,
			reason:  "Failed",
		},
		"blocked": {
			test: map[string]any{
				"last":       map[string]any{"outcome": "Passed"},
				"conditions": []any{passed("False", "SnapshotUnavailable")},
			},
			outcome: fleet.RestoreOutcomeBlocked,
			reason:  "SnapshotUnavailable",
		},
		"running": {
			test:    map[string]any{"last": map[string]any{"outcome": "Failed"}, "active": map[string]any{"name": "job"}},
			outcome: fleet.RestoreOutcomeRunning,
		},
	} {
		got := projectRestoreTest(tc.test)
		if got.Outcome != tc.outcome || (tc.reason != "" && got.Reason != tc.reason) {
			t.Fatalf("%s: outcome %s reason %q, want %s %q", name, got.Outcome, got.Reason, tc.outcome, tc.reason)
		}
	}
	digest := "sha256:" + strings.Repeat("b", 64)
	got := projectRestoreTest(map[string]any{
		"lastSuccessTime": "2026-10-06T09:00:00Z",
		"last": map[string]any{
			"outcome":    "Passed",
			"finishedAt": "2026-10-06T09:00:00Z",
			"key":        "backups/bao.snap",
			"digest":     digest,
		},
	})
	if got.LastSuccessAt == nil || got.FinishedAt == nil || got.Key == nil || got.Digest == nil || *got.Digest != digest {
		t.Fatalf("restore test evidence = %+v", got)
	}
}
