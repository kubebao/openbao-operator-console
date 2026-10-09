package evidence

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestMissingReadinessRemainsUnknown(t *testing.T) {
	now := time.Now()
	d := fleet.Deployment{ID: "test", Generation: 2}
	Evaluate(&d, now.Add(-time.Hour), false, fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: 24 * time.Hour}, now)
	if d.Health != "unknown" || len(d.Issues) != 0 {
		t.Fatalf("missing status became a concrete finding: %#v", d)
	}
}

func TestStaleSealedConditionDoesNotCreateCurrentIncident(t *testing.T) {
	now := time.Now()
	d := fleet.Deployment{
		ID:         "test",
		Generation: 2,
		Conditions: []fleet.Condition{{Type: "OpenBaoSealed", Status: "True", ObservedGeneration: 1}},
	}
	Evaluate(&d, now.Add(-time.Hour), false, fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: 24 * time.Hour}, now)
	if len(d.Issues) != 0 {
		t.Fatal("stale generation created a current sealed incident")
	}
}

func TestPauseAndStartupGraceAvoidReadinessIncident(t *testing.T) {
	now := time.Now()
	zero, three := int32(0), int32(3)
	for _, paused := range []bool{true, false} {
		d := fleet.Deployment{ID: "test", ReadyReplicas: &zero, DesiredReplicas: &three}
		Evaluate(&d, now, paused, fleet.Policy{StartupGrace: 10 * time.Minute}, now)
		if len(d.Issues) != 0 {
			t.Fatalf("paused=%t or startup grace created incident", paused)
		}
	}
}

func TestCollectionFailurePreservesCriticalEvidence(t *testing.T) {
	now := time.Now()
	d := fleet.Deployment{
		ID:     "test",
		Health: "critical",
		Issues: []fleet.Issue{{ID: "test:sealed", Rule: "sealed", Severity: "critical", FirstSeenAt: now.Add(-time.Hour)}},
	}
	CollectionFailure(&d, "denied", now)
	CollectionFailure(&d, "denied", now.Add(time.Minute))
	if d.Health != "critical" || len(d.Issues) != 2 || d.Issues[0].Rule != "sealed" {
		t.Fatal("collection failure cleared or duplicated findings")
	}
	if d.Issues[1].FirstSeenAt != now {
		t.Fatal("visibility first-seen time changed")
	}
}
