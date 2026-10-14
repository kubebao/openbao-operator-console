package collector

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

var recoveryPolicy = fleet.Policy{StartupGrace: 10 * time.Minute, MaxBackupAge: 24 * time.Hour}

// restoreRequest is an OpenBaoRestore fixture for the named cluster. An empty uid leaves
// the request unbound; an empty phase leaves status.phase absent.
func restoreRequest(name, cluster, uid, phase string, created time.Time) unstructured.Unstructured {
	status := map[string]any{}
	if phase != "" {
		status["phase"] = phase
	}
	if uid != "" {
		status["execution"] = map[string]any{"targetUID": uid}
	}
	request := unstructured.Unstructured{Object: map[string]any{
		"kind":   "OpenBaoRestore",
		"spec":   map[string]any{"cluster": cluster},
		"status": status,
	}}
	request.SetName(name)
	request.SetCreationTimestamp(metav1.NewTime(created))
	return request
}

func nativeRecovery(name, cluster, uid, phase string, created time.Time) unstructured.Unstructured {
	request := unstructured.Unstructured{Object: map[string]any{
		"kind":   "OpenBaoRecovery",
		"spec":   map[string]any{"clusterRef": map[string]any{"name": cluster}},
		"status": map[string]any{"phase": phase, "clusterRef": map[string]any{"uid": uid}},
	}}
	request.SetName(name)
	request.SetCreationTimestamp(metav1.NewTime(created))
	return request
}

func TestNewRecoveryRequestIsPendingWithinStartupGrace(t *testing.T) {
	now := time.Now().UTC()
	evaluate := func(recovery *fleet.Recovery) fleet.Deployment {
		d := fleet.Deployment{ID: "bao", Recovery: recovery}
		evidence.Evaluate(&d, now.Add(-time.Hour), false, recoveryPolicy, now)
		return d
	}
	fresh := ProjectRecovery(
		restoreRequest("fresh", "bao", "", "", now.Add(-time.Minute)),
		fleet.RecoveryMechanismSnapshot,
		now,
		recoveryPolicy.StartupGrace,
	)
	if d := evaluate(fresh); fresh.Phase != fleet.RecoveryPhasePending || len(d.Issues) != 0 {
		t.Fatalf("new request phase = %s with %d findings, want pending without findings", fresh.Phase, len(d.Issues))
	}
	late := ProjectRecovery(
		restoreRequest("late", "bao", "", "", now.Add(-time.Hour)),
		fleet.RecoveryMechanismSnapshot,
		now,
		recoveryPolicy.StartupGrace,
	)
	if d := evaluate(late); late.Phase != fleet.RecoveryPhaseUnknown || len(d.Issues) != 1 ||
		d.Issues[0].Rule != fleet.RuleRecoveryInvestigate {
		t.Fatalf("request without a phase after grace = %s with %+v", late.Phase, d.Issues)
	}
	// An explicit phase is the operator's evidence, even inside the grace.
	reported := ProjectRecovery(
		restoreRequest("reported", "bao", "", "Unknown", now),
		fleet.RecoveryMechanismSnapshot,
		now,
		recoveryPolicy.StartupGrace,
	)
	if d := evaluate(reported); reported.Phase != fleet.RecoveryPhaseUnknown || len(d.Issues) != 1 {
		t.Fatalf("reported unknown phase = %s with %d findings", reported.Phase, len(d.Issues))
	}
	unrecognized := ProjectRecovery(
		restoreRequest("odd", "bao", "", "Paused", now),
		fleet.RecoveryMechanismSnapshot,
		now,
		recoveryPolicy.StartupGrace,
	)
	if unrecognized.Phase != fleet.RecoveryPhaseUnknown {
		t.Fatalf("unrecognized phase = %s, want unknown", unrecognized.Phase)
	}
}

func TestLatestRecoveryBindsUnboundRequestsToTheClusterLifetime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	created := now.Add(-time.Hour)
	deployment := fleet.Deployment{Name: "bao", UID: "replacement"}
	latest := func(requests ...unstructured.Unstructured) *fleet.Recovery {
		return latestRecovery(requests, deployment, created, now, recoveryPolicy)
	}
	earlier := restoreRequest("for-earlier-cluster", "bao", "", "Failed", created.Add(-time.Minute))
	if got := latest(earlier); got != nil {
		t.Fatalf("a request for an earlier cluster attached to its replacement: %+v", got)
	}
	same := restoreRequest("same-second", "bao", "", "Completed", created)
	if got := latest(earlier, same); got == nil || got.RequestName != "same-second" {
		t.Fatalf("a request created with the cluster was not attached: %+v", got)
	}
	// A bound UID is authoritative in both directions.
	if got := latest(restoreRequest("other-uid", "bao", "original", "Failed", now)); got != nil {
		t.Fatal("a request bound to another UID attached to the replacement")
	}
	bound := restoreRequest("bound", "bao", "replacement", "Running", created.Add(-time.Minute))
	if got := latest(bound); got == nil || got.RequestName != "bound" {
		t.Fatal("a request bound to this UID was ignored")
	}
	if got := latest(restoreRequest("other-cluster", "other", "", "Failed", now)); got != nil {
		t.Fatal("a request for another cluster name attached")
	}
}

func TestLatestRecoverySelectsTheNewestRequestOfEitherKind(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	created := now.Add(-time.Hour)
	deployment := fleet.Deployment{Name: "bao", UID: "uid"}
	restore := restoreRequest("restore", "bao", "uid", "Completed", now.Add(-30*time.Minute))
	recovery := nativeRecovery("recovery", "bao", "uid", "Running", now.Add(-10*time.Minute))
	older := restoreRequest("older", "bao", "", "Failed", now.Add(-50*time.Minute))
	got := latestRecovery([]unstructured.Unstructured{restore, recovery, older}, deployment, created, now, recoveryPolicy)
	if got == nil || got.RequestName != "recovery" || got.Mechanism != fleet.RecoveryMechanismNative ||
		got.Phase != fleet.RecoveryPhaseRunning {
		t.Fatalf("latest recovery = %+v", got)
	}
	got = latestRecovery([]unstructured.Unstructured{older, restore}, deployment, created, now, recoveryPolicy)
	if got == nil || got.RequestName != "restore" || got.Mechanism != fleet.RecoveryMechanismSnapshot {
		t.Fatalf("latest snapshot restore = %+v", got)
	}
	if latestRecovery(nil, deployment, created, now, recoveryPolicy) != nil {
		t.Fatal("recovery evidence was fabricated without a request")
	}
}
