package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestPostgresRetainsFirstSeenAcrossRestartAndFailedCollection(t *testing.T) {
	ctx := context.Background()
	open := isolatedDatabase(t)
	p := open()
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := "test-" + now.Format("150405.000000")
	d := fleet.Deployment{
		ID:              id,
		ConnectionID:    id,
		ResourceVersion: "1",
		Health:          "critical",
		Evidence:        fleet.Evidence{Collection: "current", CollectedAt: now},
		Issues:          []fleet.Issue{{ID: id + ":sealed", Rule: "sealed", FirstSeenAt: now}},
	}
	batch := Batch{
		Connection:  fleet.Connection{ID: id, Collection: "current", LastAttemptAt: now},
		Deployments: []fleet.Deployment{d},
	}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// A status write that only changes the resource version is not a recorded change.
	churn := d
	churn.ResourceVersion = "2"
	churn.Evidence.CollectedAt = now.Add(30 * time.Second)
	batch.Connection.LastAttemptAt = now.Add(30 * time.Second)
	batch.Deployments = []fleet.Deployment{churn}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	if history, err := p.History(ctx, id); err != nil || len(history) != 1 {
		t.Fatalf("resource version churn was recorded: %v %d", err, len(history))
	}
	p.Close()
	p = open()
	defer p.Close()
	batch.Connection.LastAttemptAt = now.Add(time.Minute)
	batch.Connection.Collection = "denied"
	batch.Deployments = nil
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range snapshot.Deployments {
		if got.ID != id {
			continue
		}
		if got.Health != "critical" || got.Evidence.Collection != "denied" || len(got.Issues) != 2 ||
			!got.Issues[0].FirstSeenAt.Equal(now) {
			t.Fatalf("lost durable evidence: %#v", got)
		}
		history, err := p.History(ctx, id)
		if err != nil || len(history) != 2 {
			t.Fatalf("history not retained: %v %#v", err, history)
		}
		return
	}
	t.Fatal("last-known deployment disappeared")
}
