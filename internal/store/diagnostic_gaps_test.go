package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestDiagnosticGapsAndAcknowledgementSurviveRestartAndFailedCollection(t *testing.T) {
	storageBackends(t, diagnosticGapsAndAcknowledgementSurviveRestartAndFailedCollection)
}

func diagnosticGapsAndAcknowledgementSurviveRestartAndFailedCollection(t *testing.T) {
	ctx := context.Background()
	open := isolatedDatabase(t)
	p := open()
	now := time.Now().UTC().Truncate(time.Microsecond)
	d := observationFixture()
	d.ConnectionID, d.ID = "diagnostics", "diagnostics"
	observed := int64(2)
	d.ObservedGeneration = &observed
	d.Evidence.AnalysisIncomplete = true
	d.Evidence.AnalysisGaps = []fleet.DiagnosticGap{{
		Rule:    fleet.RuleRecoveryInvestigate,
		Reasons: []fleet.DiagnosticReason{"scope_incomplete"}, CollectionOutcome: "denied", CheckedAt: now,
	}}
	batch := Batch{Connection: fleet.Connection{
		ID: d.ConnectionID, Collection: fleet.ConnectionCollectionCurrent,
		LastAttemptAt: now,
	}, Deployments: []fleet.Deployment{d}, CompletedAt: now}
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	p.Close()
	p = open()
	defer p.Close()
	batch.Deployments, batch.CompletedAt = nil, now.Add(time.Minute)
	batch.Connection.Collection = fleet.ConnectionCollectionUnreachable
	batch.Connection.LastAttemptAt = batch.CompletedAt
	if err := p.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	snapshot, err := p.Snapshot(ctx)
	if err != nil || len(snapshot.Deployments) != 1 {
		t.Fatalf("snapshot: %v %+v", err, snapshot)
	}
	got := snapshot.Deployments[0]
	if got.ObservedGeneration == nil || *got.ObservedGeneration != 2 || len(got.Evidence.AnalysisGaps) != 1 ||
		got.Evidence.AnalysisGaps[0].CollectionOutcome != "denied" || !got.Evidence.AnalysisGaps[0].CheckedAt.Equal(now) {
		t.Fatalf("last evaluated evidence lost: %+v", got)
	}
	history, err := firstHistory(p, ctx, d.ID)
	if err != nil || len(history) != 2 || history[1].Deployment.ObservedGeneration == nil ||
		len(history[1].Deployment.Evidence.AnalysisGaps) != 1 {
		t.Fatalf("history lost projection: %v %+v", err, history)
	}
}

func TestOlderStoredProjectionDoesNotInferResourceAcknowledgement(t *testing.T) {
	var deployment fleet.Deployment
	if err := json.Unmarshal(
		[]byte(`{"generation":3,"conditions":[{"observedGeneration":3}],"evidence":{"analysisIncomplete":true}}`),
		&deployment,
	); err != nil {
		t.Fatal(err)
	}
	if deployment.ObservedGeneration != nil || len(deployment.Evidence.AnalysisGaps) != 0 || !deployment.Evidence.AnalysisIncomplete {
		t.Fatal("older projection invented new evidence")
	}
}
