package demo

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Fixture activity comes from fixture history through the rules the store applies.
func TestActivityFollowsFixtureHistory(t *testing.T) {
	repository := New(time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	sealed := false
	for i, event := range repository.activity {
		if i > 0 && event.RecordedAt.After(repository.activity[i-1].RecordedAt) {
			t.Fatal("activity is not newest first")
		}
		if event.DeploymentID == "identity" && event.Kind == fleet.ActivityFindingRaised && event.Finding.Rule == fleet.RuleSealed {
			sealed = true
		}
	}
	if !sealed {
		t.Fatal("the sealed fixture has no sealed finding event")
	}
}

func TestHistoryEndsInCurrentFixtureState(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	repository := New(now)
	snapshot, err := repository.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, deployment := range snapshot.Deployments {
		page, err := repository.History(context.Background(), deployment.ID, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
		observations := page.Observations
		if err != nil {
			t.Fatal(err)
		}
		if len(observations) == 0 {
			t.Fatalf("%s has no history", deployment.ID)
		}
		newest := observations[0].Deployment
		newest.ResourceVersion = deployment.ResourceVersion
		if !reflect.DeepEqual(newest, deployment) {
			t.Fatalf("%s history does not end in the current fixture state", deployment.ID)
		}
		for i, observation := range observations {
			if observation.RecordedAt.Before(now.Add(-7*24*time.Hour)) || observation.RecordedAt.After(now) {
				t.Fatalf("%s observation %d is outside the retention window", deployment.ID, i)
			}
			if i > 0 && observation.RecordedAt.After(observations[i-1].RecordedAt) {
				t.Fatalf("%s history is not newest first", deployment.ID)
			}
		}
	}
	for id, minimum := range map[string]int{"payments": 7, "identity": 8, "customer-x": 6, "staging": 8} {
		page, _ := repository.History(context.Background(), id, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
		observations := page.Observations
		if len(observations) < minimum {
			t.Fatalf("%s has %d observations, want at least %d", id, len(observations), minimum)
		}
	}
}

func TestViewerSeesEveryFixtureThroughNormalAuthorization(t *testing.T) {
	repository := New(time.Now().UTC())
	snapshot, err := repository.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	scoped := auth.Scope(snapshot, repository.Viewer())
	if len(scoped.Deployments) != len(snapshot.Deployments) || len(scoped.Connections) != len(snapshot.Connections) ||
		len(scoped.Relationships) != len(snapshot.Relationships) {
		t.Fatal("the demonstration viewer cannot see the whole fixture fleet")
	}
	page, err := repository.Activity(context.Background(), repository.Viewer().Scopes, fleet.Page{Limit: 500})
	events := page.Events
	if err != nil || len(events) != len(repository.activity) || len(events) == 0 {
		t.Fatalf("the demonstration viewer sees %d of %d events", len(events), len(repository.activity))
	}
	if hidden, _ := repository.Activity(context.Background(), nil, fleet.Page{Limit: 100}); len(hidden.Events) != 0 {
		t.Fatal("events leaked without a scope")
	}
	if repository.Viewer().Admin {
		t.Fatal("the demonstration viewer must not administer settings")
	}
}
