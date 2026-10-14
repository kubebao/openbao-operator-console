package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestPruneAppliesRetentionOutsideCollection(t *testing.T) {
	ctx := context.Background()
	p, err := Open(ctx, isolatedDSN(t), 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	now := time.Now().UTC().Truncate(time.Microsecond)
	old := now.Add(-48 * time.Hour)
	d := fleet.Deployment{
		ID:           "bao",
		ConnectionID: "one",
		Namespace:    "team",
		Health:       fleet.HealthHealthy,
		Issues:       []fleet.Issue{},
		Evidence:     fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, CollectedAt: old, LastAttemptAt: old},
	}
	apply := func(at time.Time, health fleet.Health) {
		t.Helper()
		current := d
		current.Health = health
		if err := p.Apply(ctx, Batch{
			Connection:  fleet.Connection{ID: "one", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: at},
			Deployments: []fleet.Deployment{current},
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(old, fleet.HealthHealthy)
	apply(old.Add(time.Minute), fleet.HealthCritical)
	apply(now, fleet.HealthHealthy)
	scope := []config.Scope{{ConnectionID: "one", Namespaces: []string{"team"}}}
	// Collection writes no longer prune.
	if history, err := p.History(ctx, "bao"); err != nil || len(history) != 3 {
		t.Fatalf("history before pruning = %d, %v", len(history), err)
	}
	if err := p.Prune(ctx, now); err != nil {
		t.Fatal(err)
	}
	history, err := p.History(ctx, "bao")
	if err != nil || len(history) != 1 || !history[0].RecordedAt.Equal(now) {
		t.Fatalf("history after pruning = %+v, %v", history, err)
	}
	events, err := p.Activity(ctx, scope)
	if err != nil || len(events) != 1 || !events[0].At.Equal(now) {
		t.Fatalf("activity after pruning = %+v, %v", events, err)
	}
	// The current inventory is not history and survives retention.
	if snapshot, err := p.Snapshot(ctx); err != nil || len(snapshot.Deployments) != 1 {
		t.Fatalf("pruning removed current inventory: %v", err)
	}
}
