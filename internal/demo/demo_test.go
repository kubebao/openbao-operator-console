package demo

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/auth"
)

func TestHistoryEndsInCurrentFixtureState(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	repository := New(now)
	snapshot, err := repository.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, deployment := range snapshot.Deployments {
		observations, err := repository.History(context.Background(), deployment.ID)
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
		observations, _ := repository.History(context.Background(), id)
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
		len(scoped.Relationships) != len(snapshot.Relationships) || len(scoped.Activity) != len(snapshot.Activity) {
		t.Fatal("the demonstration viewer cannot see the whole fixture fleet")
	}
	if repository.Viewer().Admin {
		t.Fatal("the demonstration viewer must not administer settings")
	}
}
