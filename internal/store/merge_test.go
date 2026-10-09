package store

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestFailedCollectionKeepsLastKnownDeployments(t *testing.T) {
	collected := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	attempted := collected.Add(time.Minute)
	stored := observationFixture()
	previous := fleet.Connection{ID: "cluster", LastSuccessAt: &collected}
	batch := Batch{Connection: fleet.Connection{
		ID:            "cluster",
		Collection:    fleet.ConnectionCollectionDenied,
		LastAttemptAt: attempted,
	}}
	result := merge(batch, &previous, []fleet.Deployment{stored})
	if result.complete || len(result.deployments) != 1 {
		t.Fatal("a failed collection removed or replaced stored deployments")
	}
	deployment := result.deployments[0]
	if deployment.Evidence.Collection != fleet.EvidenceCollectionDenied || !deployment.Evidence.LastAttemptAt.Equal(attempted) ||
		!deployment.Evidence.CollectedAt.Equal(collected) {
		t.Fatalf("evidence = %+v", deployment.Evidence)
	}
	if deployment.Health != fleet.HealthHealthy || deployment.Issues[len(deployment.Issues)-1].Rule != fleet.RuleCollection {
		t.Fatal("a failed collection concealed last-known state or omitted the collection finding")
	}
	if result.connection.LastSuccessAt == nil || !result.connection.LastSuccessAt.Equal(collected) {
		t.Fatal("a failed collection replaced the last successful collection time")
	}
	if len(result.observations) != 1 || len(result.activity) != 1 {
		t.Fatal("a collection state change was not recorded")
	}
}

func TestUnchangedCollectionRecordsNoHistory(t *testing.T) {
	stored := observationFixture()
	current := observationFixture()
	later := stored.Evidence.CollectedAt.Add(30 * time.Second)
	current.ResourceVersion = "101"
	current.Evidence.CollectedAt, current.Evidence.LastAttemptAt = later, later
	current.Issues[0].FirstSeenAt = later
	batch := Batch{
		Connection:  fleet.Connection{ID: "cluster", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: later},
		Deployments: []fleet.Deployment{current},
	}
	result := merge(batch, &fleet.Connection{ID: "cluster"}, []fleet.Deployment{stored})
	if !result.complete || len(result.deployments) != 1 || len(result.observations) != 0 || len(result.activity) != 0 {
		t.Fatal("status churn recorded history")
	}
	if !result.deployments[0].Issues[0].FirstSeenAt.Equal(stored.Issues[0].FirstSeenAt) {
		t.Fatal("an unresolved finding lost its first-seen time")
	}
}

func TestFirstObservationRecordsHistoryWithoutActivity(t *testing.T) {
	current := observationFixture()
	batch := Batch{
		Connection: fleet.Connection{
			ID:            "cluster",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: current.Evidence.CollectedAt,
		},
		Deployments: []fleet.Deployment{current},
	}
	result := merge(batch, nil, nil)
	if len(result.observations) != 1 || len(result.activity) != 0 {
		t.Fatal("a new deployment must record its first observation without a transition")
	}
}
