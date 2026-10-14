package evidence

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func transitionFixture() fleet.Deployment {
	return fleet.Deployment{
		ID:           "deployment",
		Name:         "payments-bao",
		ConnectionID: "cluster",
		Namespace:    "payments",
		Health:       fleet.HealthAttention,
		Evidence:     fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent},
		Issues: []fleet.Issue{
			{Rule: fleet.RuleBackupFailed, Severity: fleet.SeverityHigh, Detail: "1 consecutive failure"},
			{Rule: fleet.RuleRestoreTest, Severity: fleet.SeverityInfo},
		},
	}
}

func TestTransitionsRecordEachChangeAsStructuredEvent(t *testing.T) {
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	previous := transitionFixture()
	current := transitionFixture()
	current.Health = fleet.HealthCritical
	current.Evidence.Collection = fleet.EvidenceCollectionStale
	current.Issues = []fleet.Issue{
		{Rule: fleet.RuleSealed, Severity: fleet.SeverityCritical},
		{Rule: fleet.RuleBackupFailed, Severity: fleet.SeverityCritical, Detail: "2 consecutive failures"},
	}
	events := Transitions(previous, current, at)
	want := []struct {
		kind     fleet.ActivityKind
		rule     fleet.Rule
		severity fleet.Severity
	}{
		{kind: fleet.ActivityHealthChanged},
		{kind: fleet.ActivityCollectionChanged},
		{kind: fleet.ActivityFindingRaised, rule: fleet.RuleSealed, severity: fleet.SeverityCritical},
		{kind: fleet.ActivityFindingSeverityChanged, rule: fleet.RuleBackupFailed, severity: fleet.SeverityCritical},
		{kind: fleet.ActivityFindingResolved, rule: fleet.RuleRestoreTest, severity: fleet.SeverityInfo},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v", events)
	}
	ids := map[string]bool{}
	for i, event := range events {
		if event.Kind != want[i].kind || event.DeploymentID != "deployment" || event.DeploymentName != "payments-bao" ||
			event.ConnectionID != "cluster" || event.Namespace != "payments" || !event.RecordedAt.Equal(at) {
			t.Fatalf("event %d = %+v", i, event)
		}
		set := 0
		for _, present := range []bool{event.Health != nil, event.Collection != nil, event.Finding != nil} {
			if present {
				set++
			}
		}
		if set != 1 {
			t.Fatalf("event %d sets %d changes, want exactly one", i, set)
		}
		if want[i].rule != "" &&
			(event.Finding == nil || event.Finding.Rule != want[i].rule || event.Finding.Severity != want[i].severity) {
			t.Fatalf("event %d finding = %+v", i, event.Finding)
		}
		if ids[event.ID] {
			t.Fatalf("duplicate event ID %s", event.ID)
		}
		ids[event.ID] = true
	}
	if change := events[0].Health; change.From != fleet.HealthAttention || change.To != fleet.HealthCritical {
		t.Fatalf("health change = %+v", change)
	}
	if change := events[1].Collection; change.From != fleet.EvidenceCollectionCurrent || change.To != fleet.EvidenceCollectionStale {
		t.Fatalf("collection change = %+v", change)
	}
	if previous := events[3].Finding.PreviousSeverity; previous == nil || *previous != fleet.SeverityHigh {
		t.Fatalf("severity change lost the previous severity: %+v", events[3].Finding)
	}
	if events[2].Finding.PreviousSeverity != nil || events[4].Finding.PreviousSeverity != nil {
		t.Fatal("raised and resolved findings report a previous severity")
	}
}

func TestTransitionsIgnoreDetailOnlyFindingChanges(t *testing.T) {
	previous := transitionFixture()
	current := transitionFixture()
	current.Issues[0].Detail = "3 consecutive failures"
	if events := Transitions(previous, current, time.Now()); len(events) != 0 {
		t.Fatalf("detail change produced events: %+v", events)
	}
}
