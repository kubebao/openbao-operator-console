package store

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/contract"
)

func TestPostgresRetainsFirstSeenAcrossRestartAndFailedCollection(t *testing.T) {
	dsn := os.Getenv("CONSOLE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set CONSOLE_TEST_DATABASE_URL to an isolated console_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/console_test" {
		t.Fatal("integration test requires an isolated database named console_test")
	}
	ctx := context.Background()
	open := func() *Postgres {
		p, err := Open(ctx, dsn, 7*24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := open()
	now := time.Now().UTC().Truncate(time.Microsecond)
	id := "test-" + now.Format("150405.000000")
	d := contract.Deployment{Id: id, ConnectionId: id, ResourceVersion: "1", Health: "critical", Evidence: contract.Evidence{Collection: "current", CollectedAt: now}, Issues: []contract.Issue{{Id: id + ":sealed", Rule: "sealed", FirstSeenAt: now}}}
	batch := Batch{Connection: contract.Connection{Id: id, Collection: "current", LastAttemptAt: now}, Deployments: []contract.Deployment{d}}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	// A status write that only changes the resource version is not a recorded change.
	churn := d
	churn.ResourceVersion = "2"
	churn.Evidence.CollectedAt = now.Add(30 * time.Second)
	batch.Connection.LastAttemptAt = now.Add(30 * time.Second)
	batch.Deployments = []contract.Deployment{churn}
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
		if got.Id != id {
			continue
		}
		if got.Health != "critical" || got.Evidence.Collection != "denied" || len(got.Issues) != 2 || !got.Issues[0].FirstSeenAt.Equal(now) {
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
