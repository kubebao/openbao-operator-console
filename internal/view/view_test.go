package view

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func enrollment(connections ...config.Connection) config.Config {
	return config.Config{MaxBackupAgeHours: 24, Connections: connections}
}

func TestEnrollmentChangesImmediatelyFilterOldObservations(t *testing.T) {
	now := time.Now()
	current := enrollment(config.Connection{
		ID:         "cluster",
		Name:       "New name",
		Disabled:   true,
		Namespaces: []config.Namespace{{Name: "team", Environment: fleet.EnvironmentProduction}},
		Operators: []config.Operator{
			{ID: "operator", Namespace: "system", Deployment: "new-controller", Container: "manager", Namespaces: []string{"team"}},
		},
	})
	stored := fleet.Snapshot{
		Connections: []fleet.Connection{
			{
				ID:         "cluster",
				Namespaces: []string{"team", "removed"},
				Operators: []fleet.Operator{
					{
						ID:         "operator",
						Namespace:  "system",
						Name:       "old-controller",
						Namespaces: []string{"removed"},
						Collection: fleet.OperatorCollectionCurrent,
						Health:     fleet.OperatorHealthAvailable,
					},
				},
				Capabilities:  []fleet.Capability{{Namespace: "removed"}, {Namespace: "team"}},
				Collection:    fleet.ConnectionCollectionCurrent,
				LastAttemptAt: now,
			},
			{ID: "removed"},
		},
		Deployments: []fleet.Deployment{
			{ID: "one", ConnectionID: "cluster", Namespace: "team", Evidence: fleet.Evidence{LastAttemptAt: now}},
			{ID: "two", ConnectionID: "cluster", Namespace: "removed"},
			{ID: "three", ConnectionID: "removed"},
		},
	}
	got := Build(stored, current, now)
	if len(got.Connections) != 1 || len(got.Deployments) != 1 || got.Connections[0].Name != "New name" ||
		len(got.Connections[0].Capabilities) != 1 ||
		got.Deployments[0].Evidence.Collection != fleet.EvidenceCollectionStale {
		t.Fatal("removed enrollment or current state leaked")
	}
	operator := got.Connections[0].Operators[0]
	if operator.Name != "new-controller" || operator.Health != fleet.OperatorHealthUnknown || len(operator.Namespaces) != 1 ||
		operator.Namespaces[0] != "team" {
		t.Fatal("old controller observation inherited a new enrollment")
	}
}

func TestStoppedCollectorMakesLastKnownEvidenceStale(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-10 * time.Minute)
	current := enrollment(config.Connection{ID: "cluster", Namespaces: []config.Namespace{{Name: "team"}}})
	stored := fleet.Snapshot{
		Deployments: []fleet.Deployment{
			{
				ID:           "old",
				ConnectionID: "cluster",
				Namespace:    "team",
				Health:       fleet.HealthCritical,
				Evidence:     fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, LastAttemptAt: old},
				Issues:       []fleet.Issue{{ID: "sealed", Rule: fleet.RuleSealed}},
			},
		},
		Connections: []fleet.Connection{
			{ID: "cluster", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: old},
		},
	}
	got := Build(stored, current, now)
	deployment := got.Deployments[0]
	if deployment.Health != fleet.HealthCritical || deployment.Evidence.Collection != fleet.EvidenceCollectionStale ||
		len(deployment.Issues) != 2 {
		t.Fatal("stopped collector concealed unresolved evidence")
	}
	if got.Connections[0].Collection != fleet.ConnectionCollectionStale {
		t.Fatal("stopped collector left the connection current")
	}
	// Staleness is a state transition, not a collection attempt.
	if !deployment.Evidence.LastAttemptAt.Equal(old) {
		t.Fatalf("last attempt = %s, want the last real attempt %s", deployment.Evidence.LastAttemptAt, old)
	}
	finding := deployment.Issues[1]
	if finding.Rule != fleet.RuleCollection || !finding.FirstSeenAt.Equal(old.Add(StaleAfter)) || !finding.LastCheckedAt.Equal(now) {
		t.Fatalf("collection finding = %+v", finding)
	}
}

func TestEnrollmentIsVisibleBeforeFirstCollection(t *testing.T) {
	current := enrollment(config.Connection{ID: "new", Name: "New", Namespaces: []config.Namespace{{Name: "team"}}})
	got := Build(fleet.Snapshot{}, current, time.Now())
	if len(got.Connections) != 1 || got.Connections[0].Collection != fleet.ConnectionCollectionPending ||
		got.Connections[0].LastSuccessAt != nil {
		t.Fatal("new enrollment disappeared or claimed a successful read")
	}
}

func TestNewlyDeclaredOperatorIsPendingUntilCollected(t *testing.T) {
	now := time.Now()
	current := enrollment(config.Connection{
		ID:         "cluster",
		Namespaces: []config.Namespace{{Name: "team"}},
		Operators: []config.Operator{
			{ID: "added", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"team"}},
		},
	})
	stored := fleet.Snapshot{
		Connections: []fleet.Connection{{ID: "cluster", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: now}},
	}
	operator := Build(stored, current, now).Connections[0].Operators[0]
	if operator.Collection != fleet.OperatorCollectionPending || operator.Health != fleet.OperatorHealthUnknown {
		t.Fatalf("operator = %s/%s, want pending/unknown", operator.Collection, operator.Health)
	}
}

func TestDeclaredRelationshipsRequireVisibleEndpoints(t *testing.T) {
	now := time.Now()
	ref := func(namespace, name string) config.ResourceRef {
		return config.ResourceRef{ConnectionID: "cluster", Namespace: namespace, Name: name, UID: name + "-uid"}
	}
	deployment := func(namespace, name string) fleet.Deployment {
		return fleet.Deployment{
			ID:           fleet.DeploymentID("cluster", namespace, name, name+"-uid"),
			ConnectionID: "cluster",
			Namespace:    namespace,
			Name:         name,
			Evidence:     fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, LastAttemptAt: now},
		}
	}
	current := enrollment(config.Connection{ID: "cluster", Namespaces: []config.Namespace{{Name: "a"}, {Name: "b"}}})
	current.Relationships = []config.Relationship{
		{ID: "visible", Kind: fleet.RelationshipKindReplication, Source: ref("a", "primary"), Target: ref("b", "replica")},
		{ID: "unobserved", Kind: fleet.RelationshipKindReplication, Source: ref("a", "primary"), Target: ref("b", "missing")},
		{ID: "unenrolled", Kind: fleet.RelationshipKindReplication, Source: ref("a", "primary"), Target: ref("c", "elsewhere")},
	}
	stored := fleet.Snapshot{
		Connections: []fleet.Connection{{ID: "cluster", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: now}},
		Deployments: []fleet.Deployment{deployment("a", "primary"), deployment("b", "replica"), deployment("c", "elsewhere")},
	}
	got := Build(stored, current, now).Relationships
	if len(got) != 1 || got[0].ID != "visible" || got[0].Provenance != fleet.ProvenanceDeclared || got[0].Repository != nil {
		t.Fatalf("relationships = %+v", got)
	}
}
