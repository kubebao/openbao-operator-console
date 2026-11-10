package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Existing retention tests inspect the newest page. Pagination tests below traverse
// cursors separately, including rows with identical timestamps.
func firstActivity(p testRepository, ctx context.Context, scopes []config.Scope) ([]fleet.Activity, error) {
	page, err := p.Activity(ctx, scopes, fleet.Page{Limit: 100})
	return page.Events, err
}

func firstHistory(p testRepository, ctx context.Context, id string) ([]fleet.Observation, error) {
	page, err := p.History(ctx, id, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
	return page.Observations, err
}

func TestHistoryPagesPreserveTimestampTiesAndWindowBaseline(t *testing.T) {
	storageBackends(t, historyPagesPreserveTimestampTiesAndWindowBaseline)
}

func historyPagesPreserveTimestampTiesAndWindowBaseline(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 8 {
		recordedAt := at
		if i == 0 {
			recordedAt = at.Add(-time.Hour)
		}
		projection := fmt.Sprintf(`{"resourceVersion":"%d"}`, i)
		if err := execFixture(ctx, p,
			"INSERT INTO console_observation(deployment_id, connection_id, recorded_at, projection) VALUES ($1,$2,$3,$4)",
			"bao", "cluster", recordedAt, projection); err != nil {
			t.Fatal(err)
		}
	}
	query := fleet.HistoryQuery{Since: &at, Page: fleet.Page{Limit: 2}}
	seen := []string{}
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		page, err := p.History(ctx, "bao", query)
		if err != nil {
			t.Fatal(err)
		}
		if pageNumber == 0 && (page.Baseline == nil || page.Baseline.Deployment.ResourceVersion != "0") {
			t.Fatal("first page has no pre-window baseline")
		}
		if pageNumber > 0 && page.Baseline != nil {
			t.Fatal("a later page repeated the baseline")
		}
		for _, row := range page.Observations {
			seen = append(seen, row.Deployment.ResourceVersion)
		}
		if page.Next == nil {
			break
		}
		query.Page.After = page.Next
	}
	if fmt.Sprint(seen) != "[7 6 5 4 3 2 1]" {
		t.Fatalf("history pages lost or repeated tied rows: %v", seen)
	}
	query.Since = new(at.Add(time.Hour))
	query.Page.After = nil
	page, err := p.History(ctx, "bao", query)
	if err != nil || len(page.Observations) != 0 || page.Next != nil || page.Baseline == nil ||
		page.Baseline.Deployment.ResourceVersion != "7" {
		t.Fatalf("empty window lost its baseline: %+v, %v", page, err)
	}
}

func TestActivityPagesApplyCurrentScopeAndPreserveTimestampTies(t *testing.T) {
	storageBackends(t, activityPagesApplyCurrentScopeAndPreserveTimestampTies)
}

func activityPagesApplyCurrentScopeAndPreserveTimestampTies(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	for i := range 7 {
		insertActivity(t, p, fleet.Activity{ID: fmt.Sprintf("event-%d", i), ConnectionID: "cluster", Namespace: "visible", RecordedAt: at})
		insertActivity(
			t,
			p,
			fleet.Activity{ID: fmt.Sprintf("hidden-%d", i), ConnectionID: "cluster", Namespace: "hidden", RecordedAt: at.Add(time.Hour)},
		)
	}
	scopes := []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"visible"}}}
	query := fleet.Page{Limit: 2}
	seen := []string{}
	for pageNumber := 0; pageNumber < 10; pageNumber++ {
		page, err := p.Activity(ctx, scopes, query)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range page.Events {
			seen = append(seen, event.ID)
		}
		if page.Next == nil {
			break
		}
		query.After = page.Next
		revoked, err := p.Activity(ctx, nil, query)
		if err != nil || len(revoked.Events) != 0 || revoked.Next != nil {
			t.Fatal("a cursor retained access after the viewer lost its scopes")
		}
	}
	if fmt.Sprint(seen) != "[event-6 event-5 event-4 event-3 event-2 event-1 event-0]" {
		t.Fatalf("activity pages lost or repeated scoped rows: %v", seen)
	}
}

func TestLegacyActivityRemainsScopedWithoutInventingStructuredEvidence(t *testing.T) {
	storageBackends(t, legacyActivityRemainsScopedWithoutInventingStructuredEvidence)
}

func legacyActivityRemainsScopedWithoutInventingStructuredEvidence(t *testing.T) {
	p := isolatedDatabase(t)()
	defer p.Close()
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	const projection = `{"id":"legacy","deploymentId":"removed","connectionId":"cluster","namespace":"team",
"at":"2026-10-01T12:00:00Z","title":"old free-form title","detail":"old free-form detail","outcome":"error"}`
	if err := execFixture(ctx, p, `INSERT INTO console_activity(id, deployment_id, connection_id, namespace, recorded_at, projection)
VALUES ('legacy', 'removed', 'cluster', 'team', $1, $2)`, at, projection); err != nil {
		t.Fatal(err)
	}
	page, err := p.Activity(ctx, []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}}, fleet.Page{Limit: 10})
	if err != nil || len(page.Events) != 1 {
		t.Fatalf("legacy activity disappeared: %+v, %v", page, err)
	}
	event := page.Events[0]
	if event.Kind != fleet.ActivityLegacy || !event.RecordedAt.Equal(at) || event.DeploymentID != "removed" ||
		event.Health != nil || event.Collection != nil || event.Finding != nil {
		t.Fatal("legacy activity invented structured evidence or lost its identity")
	}
	encoded, err := json.Marshal(event)
	if err != nil || strings.Contains(string(encoded), "free-form") {
		t.Fatal("legacy prose passed through the storage adapter")
	}
	for _, projection := range []string{`{"kind":"unknown"}`, `{"kind":null}`, `{"kind":""}`} {
		if _, err := decodeActivity([]byte(projection), at); err == nil {
			t.Fatal("an invalid declared kind was treated as legacy activity")
		}
	}
}
