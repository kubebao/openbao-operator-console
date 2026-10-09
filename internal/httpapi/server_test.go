package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type viewerAuth struct {
	user    config.User
	allowed bool
}

func (a viewerAuth) Viewer(*http.Request) (config.User, bool)  { return a.user, a.allowed }
func (viewerAuth) Login(http.ResponseWriter, *http.Request)    {}
func (viewerAuth) Callback(http.ResponseWriter, *http.Request) {}
func (viewerAuth) Logout(http.ResponseWriter, *http.Request)   {}

type memoryFleet struct {
	snapshot     fleet.Snapshot
	historyReads int
}

func (m *memoryFleet) Snapshot(context.Context) (fleet.Snapshot, error) {
	return m.snapshot, nil
}

func (m *memoryFleet) History(context.Context, string) ([]fleet.Observation, error) {
	m.historyReads++
	return []fleet.Observation{}, nil
}

func TestServerAuthorizesBeforeHistoryAndFiltersEverySurface(t *testing.T) {
	now := time.Now().UTC()
	repository := &memoryFleet{snapshot: fleet.Snapshot{
		Deployments: []fleet.Deployment{
			{
				ID:           "visible",
				ConnectionID: "one",
				Namespace:    "team-a",
				Evidence:     fleet.Evidence{Collection: "current", LastAttemptAt: now},
			},
			{
				ID:           "hidden",
				ConnectionID: "one",
				Namespace:    "team-b",
				Evidence:     fleet.Evidence{Collection: "current", LastAttemptAt: now},
			},
		},
		Connections: []fleet.Connection{
			{ID: "one", Namespaces: []string{"team-a", "team-b"}, Collection: "current", LastAttemptAt: now},
		},
		Relationships: []fleet.Relationship{{SourceID: "visible", TargetID: "hidden"}},
		Activity:      []fleet.Activity{{DeploymentID: "hidden"}},
	}}
	authentication := viewerAuth{
		user:    config.User{Subject: "engineer", Scopes: []config.Scope{{ConnectionID: "one", Namespaces: []string{"team-a"}}}},
		allowed: true,
	}
	handler := (&Server{Fleet: repository, Auth: authentication}).Handler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil))
	if response.Code != 200 {
		t.Fatalf("snapshot = %d", response.Code)
	}
	var snapshot contract.Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Deployments) != 1 || len(snapshot.Relationships) != 0 || len(snapshot.Activity) != 0 ||
		len(snapshot.Connections[0].Namespaces) != 1 {
		t.Fatal("hidden inventory leaked")
	}
	hidden := httptest.NewRecorder()
	handler.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/hidden/history", nil))
	if hidden.Code != 404 || repository.historyReads != 0 {
		t.Fatal("history read before authorization")
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(method, "/api/v1/snapshot", nil))
		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s API = %d", method, w.Code)
		}
	}
	unauthorized := (&Server{Fleet: repository, Auth: viewerAuth{allowed: false}}).Handler()
	w := httptest.NewRecorder()
	unauthorized.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/snapshot", nil))
	if w.Code != 401 {
		t.Fatal("anonymous live API request accepted")
	}
}
