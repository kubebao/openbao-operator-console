package store

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func observationFixture() fleet.Deployment {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	ready, desired := int32(3), int32(3)
	return fleet.Deployment{
		ID:              "cluster/team-a/bao",
		ResourceVersion: "100",
		Generation:      3,
		Health:          "healthy",
		ReadyReplicas:   &ready,
		DesiredReplicas: &desired,
		Evidence:        fleet.Evidence{Collection: "current", CollectedAt: now, LastAttemptAt: now, ProducerFreshness: "unknown"},
		Conditions:      []fleet.Condition{{Type: "Available", Status: "True", Reason: "Ready", ObservedGeneration: 3}},
		Replica:         &fleet.Replica{ObservedAt: &now, LocalIndex: ptrString("10"), SourceIndex: ptrString("12")},
		Issues: []fleet.Issue{
			{
				ID:            "cluster/team-a/bao:backup-failed",
				Rule:          "backup-failed",
				Severity:      "high",
				Category:      "backup",
				Title:         "Backup failed",
				Detail:        "1",
				FirstSeenAt:   now,
				LastCheckedAt: now,
			},
		},
	}
}

func ptrString(value string) *string { return &value }

func TestObservationChangedIgnoresStatusChurn(t *testing.T) {
	previous := observationFixture()
	current := observationFixture()
	later := previous.Evidence.CollectedAt.Add(30 * time.Second)
	current.ResourceVersion = "101"
	current.Evidence.CollectedAt, current.Evidence.LastAttemptAt = later, later
	current.Replica = &fleet.Replica{ObservedAt: &later, LocalIndex: ptrString("11"), SourceIndex: ptrString("13")}
	current.Issues[0].LastCheckedAt = later
	if observationChanged(current, previous) {
		t.Fatal("resource version, collection times, replica progress, and check times must not record an observation")
	}
}

func TestObservationChangedDetectsDisplayedChanges(t *testing.T) {
	for name, change := range map[string]func(*fleet.Deployment){
		"ready replicas":      func(d *fleet.Deployment) { ready := int32(2); d.ReadyReplicas = &ready },
		"condition":           func(d *fleet.Deployment) { d.Conditions[0].Status = "False" },
		"health":              func(d *fleet.Deployment) { d.Health = "attention" },
		"collection":          func(d *fleet.Deployment) { d.Evidence.Collection = "unreachable" },
		"finding detail":      func(d *fleet.Deployment) { d.Issues[0].Detail = "2" },
		"generation":          func(d *fleet.Deployment) { d.Generation = 4 },
		"observed generation": func(d *fleet.Deployment) { value := int64(3); d.ObservedGeneration = &value },
		"analysis gap": func(d *fleet.Deployment) {
			d.Evidence.AnalysisIncomplete = true
			d.Evidence.AnalysisGaps = []fleet.DiagnosticGap{{Rule: fleet.RuleSealed, Reasons: []fleet.DiagnosticReason{"missing_field"}}}
		},
		"history conflict": func(d *fleet.Deployment) {
			d.Replica = &fleet.Replica{
				ObservedAt:      d.Replica.ObservedAt,
				LocalIndex:      d.Replica.LocalIndex,
				SourceIndex:     d.Replica.SourceIndex,
				HistoryConflict: true,
			}
		},
	} {
		previous := observationFixture()
		current := observationFixture()
		change(&current)
		if !observationChanged(current, previous) {
			t.Fatalf("%s change was not recorded", name)
		}
	}
}

func TestObservationIgnoresGapCheckTimeButRecordsChangedCause(t *testing.T) {
	previous, current := observationFixture(), observationFixture()
	previous.Evidence.AnalysisGaps = []fleet.DiagnosticGap{{
		Rule:    fleet.RuleSealed,
		Reasons: []fleet.DiagnosticReason{"missing_field"}, CollectionOutcome: "complete", CheckedAt: time.Now().UTC(),
	}}
	current.Evidence.AnalysisGaps = append([]fleet.DiagnosticGap{}, previous.Evidence.AnalysisGaps...)
	current.Evidence.AnalysisGaps[0].CheckedAt = previous.Evidence.AnalysisGaps[0].CheckedAt.Add(time.Minute)
	if observationChanged(current, previous) {
		t.Fatal("analysis time alone created history")
	}
	current.Evidence.AnalysisGaps[0].Reasons = []fleet.DiagnosticReason{"generation_unconfirmed"}
	if !observationChanged(current, previous) {
		t.Fatal("changed gap cause was not recorded")
	}
}
