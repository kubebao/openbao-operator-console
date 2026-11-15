package collector

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func normalizedRestore(t *testing.T, name, uid, phase string) shared.Restore {
	t.Helper()
	status := map[string]any{}
	if phase != "" {
		status["phase"] = phase
	}
	if uid != "" {
		status["execution"] = map[string]any{"targetUID": uid}
	}
	data, err := json.Marshal(
		map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoRestore",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "team",
				"uid":               name + "-uid",
				"creationTimestamp": time.Now().UTC().Format(time.RFC3339),
			},
			"spec":   map[string]any{"cluster": "bao"},
			"status": status,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ProjectRestore(
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

func TestRecoveryRequiresUIDBindingAndPreservesUnknownPhase(t *testing.T) {
	cluster := normalizedCluster(t, testCluster())
	for _, uid := range []string{"", "other", "original"} {
		d := fleet.Deployment{}
		attachRecovery(&d, cluster, []shared.Restore{normalizedRestore(t, "restore", uid, "")})
		if uid == "original" {
			if d.Recovery == nil || d.Recovery.Phase != fleet.RecoveryPhaseUnknown || d.Recovery.ApplicationConfirmedAt != nil ||
				d.Recovery.Cleanup != fleet.RecoveryCleanupUnknown {
				t.Fatalf("missing phase became acceptance or application: %+v", d.Recovery)
			}
		} else if d.Recovery != nil {
			t.Fatal("name or creation time became ownership proof")
		}
	}
}
