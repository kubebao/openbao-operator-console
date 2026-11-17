package store

import (
	"context"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestConditionFindingsRetainIndependentEpisodesAcrossRestart(t *testing.T) {
	storageBackends(t, conditionFindingsRetainIndependentEpisodesAcrossRestart)
}

func conditionFindingsRetainIndependentEpisodesAcrossRestart(t *testing.T) {
	ctx := context.Background()
	open := isolatedDatabase(t)
	p := open()
	now := time.Now().UTC().Truncate(time.Microsecond)
	d := observationFixture()
	d.ID, d.ConnectionID, d.Health = "conditions", "conditions", fleet.HealthAttention
	d.Issues = []fleet.Issue{}
	for _, condition := range []string{"Degraded", "TLSReady"} {
		d.Issues = append(d.Issues, fleet.Issue{
			ID: d.ID + ":operator-condition:" + condition, DeploymentID: d.ID,
			Rule: fleet.RuleOperatorCondition, ConditionType: condition, Severity: fleet.SeverityHigh,
			FirstSeenAt: now, LastCheckedAt: now,
		})
	}
	batch := Batch{
		Connection:  fleet.Connection{ID: d.ConnectionID, Collection: fleet.ConnectionCollectionCurrent, LastAttemptAt: now},
		Deployments: []fleet.Deployment{d}, CompletedAt: now,
	}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	p.Close()
	p = open()
	defer p.Close()

	current := d
	current.Evidence.AnalysisIncomplete = true
	current.Issues = append([]fleet.Issue{}, d.Issues[:1]...)
	current.Issues[0].FirstSeenAt, current.Issues[0].LastCheckedAt = now.Add(time.Minute), now.Add(time.Minute)
	batch.Deployments, batch.CompletedAt = []fleet.Deployment{current}, now.Add(time.Minute)
	batch.Unresolved = map[string][]fleet.Rule{d.ID: {fleet.RuleOperatorCondition}}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Snapshot(ctx)
	if err != nil || len(snapshot.Deployments) != 1 || len(snapshot.Deployments[0].Issues) != 2 {
		t.Fatalf("independent condition episodes were not retained: %v %+v", err, snapshot.Deployments)
	}
	for _, issue := range snapshot.Deployments[0].Issues {
		if issue.ConditionType == "" || !issue.FirstSeenAt.Equal(now) ||
			issue.ConditionType == "TLSReady" && !issue.LastCheckedAt.Equal(now) {
			t.Fatalf("condition identity or last confirmed evaluation changed: %+v", issue)
		}
	}

	current.Evidence.AnalysisIncomplete = false
	current.Issues = append([]fleet.Issue{}, d.Issues[1:]...)
	current.Issues[0].FirstSeenAt, current.Issues[0].LastCheckedAt = now.Add(2*time.Minute), now.Add(2*time.Minute)
	batch.Deployments, batch.CompletedAt, batch.Unresolved = []fleet.Deployment{current}, now.Add(2*time.Minute), nil
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	history, err := firstHistory(p, ctx, d.ID)
	if err != nil || len(history) != 3 || len(history[0].Deployment.Issues) != 1 {
		t.Fatalf("condition resolution did not produce history: %v %+v", err, history)
	}
	remaining := history[0].Deployment.Issues[0]
	if remaining.ConditionType != "TLSReady" || !remaining.FirstSeenAt.Equal(now) {
		t.Fatalf("resolving another condition reset the retained episode: %+v", remaining)
	}
	events, err := firstActivity(p, ctx, []config.Scope{{ConnectionID: d.ConnectionID, Namespaces: []string{d.Namespace}}})
	if err != nil || len(events) != 1 || events[0].Kind != fleet.ActivityFindingResolved || events[0].Finding == nil ||
		events[0].Finding.ConditionType != "Degraded" {
		t.Fatalf("resolved condition identity was lost from activity: %v %+v", err, events)
	}
}
