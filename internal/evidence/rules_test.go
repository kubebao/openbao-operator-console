package evidence

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestMapsSharedFindingsWithStableProductIdentity(t *testing.T) {
	now := time.Now().UTC()
	d := fleet.Deployment{ID: "deployment"}
	report := diagnose.Report{
		Policy:   diagnose.Policy{ReferenceTime: now},
		Findings: []diagnose.Finding{{Rule: diagnose.RuleSealed, Severity: diagnose.Critical, Summary: "OpenBao is sealed."}},
	}
	Apply(&d, report)
	if len(d.Issues) != 1 || d.Issues[0].ID != "deployment:"+string(fleet.RuleSealed) || d.Health != fleet.HealthCritical ||
		d.Issues[0].FirstSeenAt != now {
		t.Fatalf("mapping: %+v", d)
	}
}

func TestRestoreFindingsHaveIndependentIdentities(t *testing.T) {
	d := fleet.Deployment{ID: "deployment"}
	report := diagnose.Report{Policy: diagnose.Policy{ReferenceTime: time.Now().UTC()}}
	for _, name := range []string{"first", "second"} {
		report.Findings = append(
			report.Findings,
			diagnose.Finding{
				Rule:     diagnose.RuleRestore,
				Severity: diagnose.Warning,
				Summary:  "Restore failed.",
				Subject: shared.Identity{
					Origin:    "one",
					Group:     "openbao.org",
					Kind:      "OpenBaoRestore",
					Namespace: "team",
					Name:      name,
					UID:       name + "-uid",
				},
			},
		)
	}
	Apply(&d, report)
	if len(d.Issues) != 2 || d.Issues[0].ID == d.Issues[1].ID {
		t.Fatalf("request identity collapsed: %+v", d.Issues)
	}
}

func TestConditionFindingsKeepIndependentStableIdentities(t *testing.T) {
	d := fleet.Deployment{ID: "deployment"}
	report := diagnose.Report{Policy: diagnose.Policy{ReferenceTime: time.Now().UTC()}}
	for _, condition := range []string{"Degraded", "TLSReady"} {
		report.Findings = append(report.Findings, diagnose.Finding{
			Rule: diagnose.RuleCondition, Severity: diagnose.Warning, Summary: "Current adverse condition.", Subsubject: condition,
			Subject: shared.Identity{
				Origin: "one", Group: "kubebao.org", Kind: "OpenBaoCluster", Namespace: "team", Name: "bao", UID: "cluster",
			},
		})
	}
	Apply(&d, report)
	if len(d.Issues) != 2 || d.Issues[0].ID == d.Issues[1].ID || d.Issues[0].ConditionType != "Degraded" ||
		d.Issues[1].ConditionType != "TLSReady" {
		t.Fatalf("condition identities collapsed: %+v", d.Issues)
	}
	id := d.Issues[0].ID
	report.Findings[0].Severity, report.Findings[0].Summary = diagnose.Info, "Changed wording."
	report.Policy.ReferenceTime = report.Policy.ReferenceTime.Add(time.Minute)
	Apply(&d, report)
	if d.Issues[0].ID != id {
		t.Fatal("severity, wording, or reference time changed condition identity")
	}
}

func TestInsufficientCoverageCannotBecomeHealthy(t *testing.T) {
	ready := int32(3)
	d := fleet.Deployment{ID: "deployment", ReadyReplicas: &ready, DesiredReplicas: &ready}
	unresolved := Apply(&d, diagnose.Report{Coverage: []diagnose.Coverage{{Rule: diagnose.RuleRestore, State: diagnose.Insufficient}}})
	if !d.Evidence.AnalysisIncomplete || d.Health != fleet.HealthUnknown || len(unresolved) != 1 ||
		unresolved[0] != fleet.RuleRecoveryInvestigate {
		t.Fatalf("incomplete mapping: %+v", d)
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
		Apply(&d, diagnose.Report{Policy: diagnose.Policy{ReferenceTime: now}})
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
