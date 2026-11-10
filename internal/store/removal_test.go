package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestRemovedConnectionIsForgottenAndReenrollsEmpty(t *testing.T) {
	storageBackends(t, removedConnectionIsForgottenAndReenrollsEmpty)
}

func removedConnectionIsForgottenAndReenrollsEmpty(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	deployment := func(connection string, health fleet.Health) fleet.Deployment {
		return fleet.Deployment{
			ID:           connection + "-bao",
			ConnectionID: connection,
			Namespace:    "team",
			Health:       health,
			Issues:       []fleet.Issue{},
			Evidence:     fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, CollectedAt: now, LastAttemptAt: now},
		}
	}
	apply := func(connection string, collection fleet.ConnectionCollection, at time.Time, deployments ...fleet.Deployment) {
		t.Helper()
		if err := p.Apply(ctx, Batch{
			Connection:  fleet.Connection{ID: connection, Collection: collection, LastAttemptAt: at},
			Deployments: deployments,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, connection := range []string{"kept", "removed"} {
		apply(connection, fleet.ConnectionCollectionCurrent, now, deployment(connection, fleet.HealthHealthy))
		apply(connection, fleet.ConnectionCollectionCurrent, now.Add(time.Minute), deployment(connection, fleet.HealthCritical))
	}
	if err := p.RetainConnections(ctx, []string{"kept", "paused"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Connections) != 1 || snapshot.Connections[0].ID != "kept" ||
		len(snapshot.Deployments) != 1 || snapshot.Deployments[0].ConnectionID != "kept" {
		t.Fatalf("removed connection inventory remains: %+v", snapshot)
	}
	scopes := func(connection string) []config.Scope {
		return []config.Scope{{ConnectionID: connection, Namespaces: []string{"team"}}}
	}
	if history, err := firstHistory(p, ctx, "removed-bao"); err != nil || len(history) != 0 {
		t.Fatalf("removed connection history remains: %v %d", err, len(history))
	}
	if events, err := firstActivity(p, ctx, scopes("removed")); err != nil || len(events) != 0 {
		t.Fatalf("removed connection activity remains: %v %d", err, len(events))
	}
	if history, err := firstHistory(p, ctx, "kept-bao"); err != nil || len(history) != 2 {
		t.Fatalf("enrolled connection history changed: %v %d", err, len(history))
	}
	if events, err := firstActivity(p, ctx, scopes("kept")); err != nil || len(events) != 1 {
		t.Fatalf("enrolled connection activity changed: %v %d", err, len(events))
	}
	// A re-enrolled ID whose first collection fails has no last-known deployments.
	apply("removed", fleet.ConnectionCollectionUnreachable, now.Add(2*time.Minute))
	snapshot, err = p.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range snapshot.Deployments {
		if d.ConnectionID == "removed" {
			t.Fatal("re-enrollment resurrected a deployment of the removed connection")
		}
	}
	for _, c := range snapshot.Connections {
		if c.ID == "removed" && c.LastSuccessAt != nil {
			t.Fatal("re-enrollment inherited the last successful collection")
		}
	}
}
