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

func TestIncompleteCollectionIsNeverHealthy(t *testing.T) {
	now := time.Now()
	ready := int32(3)
	for name, mark := range map[string]func(*fleet.Deployment){
		"failure": func(d *fleet.Deployment) { CollectionFailure(d, fleet.EvidenceCollectionUnreachable, now) },
		"stale":   func(d *fleet.Deployment) { Stale(d, now.Add(-time.Minute), now) },
	} {
		d := fleet.Deployment{ID: "test", ReadyReplicas: &ready, DesiredReplicas: &ready}
		Evaluate(&d, now.Add(-time.Hour), false, fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: time.Hour}, now)
		if d.Health != fleet.HealthHealthy {
			t.Fatalf("%s: fixture health = %s", name, d.Health)
		}
		mark(&d)
		mark(&d)
		if d.Health != fleet.HealthAttention || len(d.Issues) != 1 || d.Issues[0].Rule != fleet.RuleCollection {
			t.Fatalf("%s: health = %s with %d findings, want attention with one collection finding", name, d.Health, len(d.Issues))
		}
	}
}

func TestReplicaReadinessWaitsForStartupGrace(t *testing.T) {
	now := time.Now()
	policy := fleet.Policy{StartupGrace: 10 * time.Minute, MaxBackupAge: 24 * time.Hour}
	desired := int32(3)
	for name, tc := range map[string]struct {
		ready    int32
		age      time.Duration
		severity fleet.Severity
		health   fleet.Health
	}{
		"zero ready inside grace":  {ready: 0, age: 5 * time.Minute, health: fleet.HealthUnknown},
		"zero ready after grace":   {ready: 0, age: 11 * time.Minute, severity: fleet.SeverityCritical, health: fleet.HealthCritical},
		"some ready after grace":   {ready: 2, age: time.Hour, severity: fleet.SeverityHigh, health: fleet.HealthAttention},
		"all ready after grace":    {ready: 3, age: time.Hour, health: fleet.HealthHealthy},
		"more ready during change": {ready: 4, age: time.Hour, health: fleet.HealthHealthy},
	} {
		ready := tc.ready
		d := fleet.Deployment{ID: "test", ReadyReplicas: &ready, DesiredReplicas: &desired}
		Evaluate(&d, now.Add(-tc.age), false, policy, now)
		if d.Health != tc.health {
			t.Fatalf("%s: health = %s, want %s", name, d.Health, tc.health)
		}
		if tc.severity == "" {
			if len(d.Issues) != 0 {
				t.Fatalf("%s: unexpected findings %+v", name, d.Issues)
			}
			continue
		}
		if len(d.Issues) != 1 || d.Issues[0].Rule != fleet.RuleReplicaReadiness || d.Issues[0].Severity != tc.severity {
			t.Fatalf("%s: findings = %+v", name, d.Issues)
		}
	}
}

func TestBackupAgeReportsTheFirstApplicableFinding(t *testing.T) {
	now := time.Now()
	policy := fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: 24 * time.Hour}
	recent, old := now.Add(-time.Hour), now.Add(-25*time.Hour)
	for name, tc := range map[string]struct {
		backup *fleet.Backup
		rule   fleet.Rule
	}{
		"not configured":         {backup: nil},
		"recent success":         {backup: &fleet.Backup{LastSuccessAt: &recent}},
		"never succeeded":        {backup: &fleet.Backup{}, rule: fleet.RuleBackupNeverSucceeded},
		"overdue":                {backup: &fleet.Backup{LastSuccessAt: &old}, rule: fleet.RuleBackupOverdue},
		"failed after success":   {backup: &fleet.Backup{LastSuccessAt: &recent, ConsecutiveFailures: 1}, rule: fleet.RuleBackupFailed},
		"failure before overdue": {backup: &fleet.Backup{LastSuccessAt: &old, ConsecutiveFailures: 3}, rule: fleet.RuleBackupFailed},
	} {
		d := fleet.Deployment{ID: "test", Backup: tc.backup}
		Evaluate(&d, now.Add(-time.Hour), false, policy, now)
		if tc.rule == "" {
			if len(d.Issues) != 0 {
				t.Fatalf("%s: unexpected findings %+v", name, d.Issues)
			}
			continue
		}
		if len(d.Issues) != 1 || d.Issues[0].Rule != tc.rule || d.Issues[0].Severity != fleet.SeverityHigh ||
			d.Health != fleet.HealthAttention {
			t.Fatalf("%s: findings = %+v, health %s; want %s", name, d.Issues, d.Health, tc.rule)
		}
	}
}
