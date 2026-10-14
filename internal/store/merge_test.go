package store

import (
	"slices"
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
	if deployment.Health != fleet.HealthAttention || deployment.Issues[len(deployment.Issues)-1].Rule != fleet.RuleCollection {
		t.Fatal("a failed collection reported healthy or omitted the collection finding")
	}
	if *deployment.ReadyReplicas != 3 || deployment.Issues[0].Rule != fleet.RuleBackupFailed {
		t.Fatal("a failed collection concealed last-known state")
	}
	if result.connection.LastSuccessAt == nil || !result.connection.LastSuccessAt.Equal(collected) {
		t.Fatal("a failed collection replaced the last successful collection time")
	}
	if len(result.observations) != 1 ||
		!slices.Equal(kinds(result.activity), []string{"healthChanged", "collectionChanged", "findingRaised:collection"}) {
		t.Fatalf("a collection state change was not recorded: %v", kinds(result.activity))
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

func TestCollectionLossAndRecoveryRecordTransitions(t *testing.T) {
	stored := observationFixture()
	stored.Issues = []fleet.Issue{}
	attempted := stored.Evidence.CollectedAt.Add(time.Minute)
	failed := merge(Batch{Connection: fleet.Connection{
		ID:            "cluster",
		Collection:    fleet.ConnectionCollectionUnreachable,
		LastAttemptAt: attempted,
	}}, &fleet.Connection{ID: "cluster"}, []fleet.Deployment{stored})
	if got := kinds(failed.activity); !slices.Equal(got, []string{"healthChanged", "collectionChanged", "findingRaised:collection"}) {
		t.Fatalf("collection loss activity = %v", got)
	}
	if change := failed.activity[1].Collection; change == nil || change.From != fleet.EvidenceCollectionCurrent ||
		change.To != fleet.EvidenceCollectionUnreachable || !failed.activity[1].RecordedAt.Equal(attempted) {
		t.Fatalf("collection change = %+v", failed.activity[1])
	}
	recovered := stored
	recovered.Evidence.CollectedAt, recovered.Evidence.LastAttemptAt = attempted.Add(time.Minute), attempted.Add(time.Minute)
	result := merge(Batch{
		Connection: fleet.Connection{
			ID:            "cluster",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: attempted.Add(time.Minute),
		},
		Deployments: []fleet.Deployment{recovered},
	}, &failed.connection, failed.deployments)
	if got := kinds(result.activity); !slices.Equal(got, []string{"healthChanged", "collectionChanged", "findingResolved:collection"}) {
		t.Fatalf("collection recovery activity = %v", got)
	}
	if change := result.activity[0].Health; change == nil || change.From != fleet.HealthAttention || change.To != fleet.HealthHealthy {
		t.Fatalf("health change = %+v", result.activity[0])
	}
}

// kinds lists event kinds, with the rule of finding events.
func kinds(events []fleet.Activity) []string {
	result := []string{}
	for _, event := range events {
		kind := string(event.Kind)
		if event.Finding != nil {
			kind += ":" + string(event.Finding.Rule)
		}
		result = append(result, kind)
	}
	return result
}

// A displayed change without a health, collection, or finding change records history
// but no activity.
func TestObservationWithoutTransitionRecordsNoActivity(t *testing.T) {
	stored := observationFixture()
	current := observationFixture()
	later := stored.Evidence.CollectedAt.Add(30 * time.Second)
	current.Evidence.CollectedAt, current.Evidence.LastAttemptAt = later, later
	current.Version = "2.7.1"
	batch := Batch{
		Connection:  fleet.Connection{ID: "cluster", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: later},
		Deployments: []fleet.Deployment{current},
	}
	result := merge(batch, &fleet.Connection{ID: "cluster"}, []fleet.Deployment{stored})
	if len(result.observations) != 1 || len(result.activity) != 0 {
		t.Fatalf("version change recorded %d observations and %d events", len(result.observations), len(result.activity))
	}
	// A finding detail change, such as a failure counter, is history without an event.
	counted := observationFixture()
	counted.Issues[0].Detail = "2"
	batch.Deployments = []fleet.Deployment{counted}
	result = merge(batch, &fleet.Connection{ID: "cluster"}, []fleet.Deployment{stored})
	if len(result.observations) != 1 || len(result.activity) != 0 {
		t.Fatalf("detail change recorded %d observations and %d events", len(result.observations), len(result.activity))
	}
	changed := observationFixture()
	changed.ConnectionID, changed.Namespace = "cluster", "team-a"
	changed.Issues[0].Severity = fleet.SeverityCritical
	batch.Deployments = []fleet.Deployment{changed}
	result = merge(batch, &fleet.Connection{ID: "cluster"}, []fleet.Deployment{stored})
	if len(result.activity) != 1 || result.activity[0].Kind != fleet.ActivityFindingSeverityChanged ||
		result.activity[0].ConnectionID != "cluster" || result.activity[0].Namespace != "team-a" {
		t.Fatalf("finding change activity = %+v", result.activity)
	}
}
