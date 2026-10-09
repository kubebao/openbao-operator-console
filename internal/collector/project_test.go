package collector

import (
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
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
	d := Project(
		cluster,
		config.Connection{ID: "one"},
		config.Namespace{Team: "Platform", Environment: "production"},
		evidence.Policy{},
		time.Now(),
	)
	if d.Replica == nil || d.Replica.LocalIndex == nil || *d.Replica.LocalIndex != "9007199254741007" {
		t.Fatal("exact index was lost")
	}
	if d.ReadyReplicas != nil || d.Health != "unknown" {
		t.Fatal("missing readiness became zero or healthy")
	}
	if strings.Contains(d.Conditions[0].Message, "do-not-store") {
		t.Fatal("raw provider message leaked")
	}
	if ResourceID("one", "team", "bao", "original") == ResourceID("one", "team", "bao", "replacement") {
		t.Fatal("replacement inherited identity")
	}
}
