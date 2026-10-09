package store

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/contract"
)

func observationFixture() contract.Deployment {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	ready, desired := int32(3), int32(3)
	return contract.Deployment{
		Id:              "cluster/team-a/bao",
		ResourceVersion: "100",
		Generation:      3,
		Health:          "healthy",
		ReadyReplicas:   &ready,
		DesiredReplicas: &desired,
		Evidence:        contract.Evidence{Collection: "current", CollectedAt: now, LastAttemptAt: now, ProducerFreshness: "unknown"},
		Conditions:      []contract.Condition{{Type: "Available", Status: "True", Reason: "Ready", ObservedGeneration: 3}},
		Replica:         &contract.Replica{ObservedAt: &now, LocalIndex: ptrString("10"), SourceIndex: ptrString("12")},
		Issues: []contract.Issue{
			{
				Id:            "cluster/team-a/bao:backup-failed",
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
	current.Replica = &contract.Replica{ObservedAt: &later, LocalIndex: ptrString("11"), SourceIndex: ptrString("13")}
	current.Issues[0].LastCheckedAt = later
	if ObservationChanged(current, previous) {
		t.Fatal("resource version, collection times, replica progress, and check times must not record an observation")
	}
}

func TestObservationChangedDetectsDisplayedChanges(t *testing.T) {
	for name, change := range map[string]func(*contract.Deployment){
		"ready replicas": func(d *contract.Deployment) { ready := int32(2); d.ReadyReplicas = &ready },
		"condition":      func(d *contract.Deployment) { d.Conditions[0].Status = "False" },
		"health":         func(d *contract.Deployment) { d.Health = "attention" },
		"collection":     func(d *contract.Deployment) { d.Evidence.Collection = "unreachable" },
		"finding detail": func(d *contract.Deployment) { d.Issues[0].Detail = "2" },
		"generation":     func(d *contract.Deployment) { d.Generation = 4 },
		"history conflict": func(d *contract.Deployment) {
			d.Replica = &contract.Replica{
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
		if !ObservationChanged(current, previous) {
			t.Fatalf("%s change was not recorded", name)
		}
	}
}
