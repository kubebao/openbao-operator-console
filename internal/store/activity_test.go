package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store/database"
)

func insertActivity(t *testing.T, p *Postgres, activity fleet.Activity) {
	t.Helper()
	if activity.Kind == "" {
		activity.Kind = fleet.ActivityLegacy
	}
	projection, err := json.Marshal(activity)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.New(p.pool).InsertActivity(context.Background(), database.InsertActivityParams{
		ID:           activity.ID,
		DeploymentID: activity.DeploymentID,
		ConnectionID: activity.ConnectionID,
		Namespace:    activity.Namespace,
		RecordedAt:   pgtype.Timestamptz{Time: activity.RecordedAt, Valid: true},
		Projection:   projection,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestScopedViewerReceivesNewestEventsInScope(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	for i := range 120 {
		insertActivity(t, p, fleet.Activity{
			ID: fmt.Sprintf("visible-%03d", i), DeploymentID: "visible", ConnectionID: "one", Namespace: "team-a",
			RecordedAt: base.Add(time.Duration(i) * time.Second),
		})
	}
	// Newer events in another namespace and another connection would fill a fleet-wide limit.
	for i := range 150 {
		at := base.Add(time.Duration(1000+i) * time.Second)
		insertActivity(t, p, fleet.Activity{
			ID: fmt.Sprintf("namespace-%03d", i), DeploymentID: "hidden", ConnectionID: "one", Namespace: "team-b", RecordedAt: at,
		})
		insertActivity(t, p, fleet.Activity{
			ID: fmt.Sprintf("connection-%03d", i), DeploymentID: "other", ConnectionID: "two", Namespace: "team-a", RecordedAt: at,
		})
	}
	events, err := firstActivity(p, ctx, []config.Scope{{ConnectionID: "one", Namespaces: []string{"team-a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 100 {
		t.Fatalf("scoped viewer received %d events, want 100", len(events))
	}
	for i, event := range events {
		if want := fmt.Sprintf("visible-%03d", 119-i); event.ID != want {
			t.Fatalf("event %d = %s, want %s", i, event.ID, want)
		}
	}
	// Two scopes combine; a scope pair never matches a namespace of another connection.
	events, err = firstActivity(p, ctx, []config.Scope{
		{ConnectionID: "one", Namespaces: []string{"team-a"}},
		{ConnectionID: "two", Namespaces: []string{"team-b"}},
	})
	if err != nil || len(events) != 100 || events[0].ID != "visible-119" {
		t.Fatalf("combined scopes = %v, %d events", err, len(events))
	}
	if events, err := firstActivity(p, ctx, nil); err != nil || len(events) != 0 {
		t.Fatalf("a viewer without scopes received %d events: %v", len(events), err)
	}
}

func TestEventsOfDeletedDeploymentsRemainInScope(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	ready, desired := int32(1), int32(1)
	d := fleet.Deployment{
		ID:              "removed-deployment",
		ConnectionID:    "one",
		Namespace:       "team-a",
		ReadyReplicas:   &ready,
		DesiredReplicas: &desired,
		Health:          fleet.HealthHealthy,
		Issues:          []fleet.Issue{},
		Evidence:        fleet.Evidence{Collection: fleet.EvidenceCollectionCurrent, CollectedAt: now, LastAttemptAt: now},
	}
	apply := func(at time.Time, deployments ...fleet.Deployment) {
		t.Helper()
		if err := p.Apply(ctx, Batch{
			Connection:  fleet.Connection{ID: "one", Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: at},
			Deployments: deployments,
		}); err != nil {
			t.Fatal(err)
		}
	}
	apply(now, d)
	changed := d
	changed.Health = fleet.HealthCritical
	apply(now.Add(time.Minute), changed)
	apply(now.Add(2 * time.Minute))
	snapshot, err := p.Snapshot(ctx)
	if err != nil || len(snapshot.Deployments) != 0 {
		t.Fatalf("deployment was not deleted: %v", err)
	}
	events, err := firstActivity(p, ctx, []config.Scope{{ConnectionID: "one", Namespaces: []string{"team-a"}}})
	if err != nil || len(events) != 1 || events[0].DeploymentID != d.ID || events[0].Namespace != "team-a" {
		t.Fatalf("events of a deleted deployment = %+v, %v", events, err)
	}
	if hidden, err := firstActivity(p, ctx, []config.Scope{{ConnectionID: "one", Namespaces: []string{"team-b"}}}); err != nil ||
		len(hidden) != 0 {
		t.Fatalf("out-of-scope viewer received %d events: %v", len(hidden), err)
	}
}
