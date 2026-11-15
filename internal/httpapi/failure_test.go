package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// driverError imitates a PostgreSQL error whose message contains connection details.
type driverError struct{}

func (driverError) Error() string {
	return "FATAL: password authentication failed for user secret-user"
}
func (driverError) SQLState() string { return "28P01" }

type failingFleet struct {
	snapshot fleet.Snapshot
	history  error
}

func (f failingFleet) Snapshot(context.Context) (fleet.Snapshot, error) {
	if f.history == nil {
		return fleet.Snapshot{}, fmt.Errorf("read snapshot: %w", driverError{})
	}
	return f.snapshot, nil
}

func (f failingFleet) Activity(context.Context, []config.Scope, fleet.Page) (fleet.ActivityPage, error) {
	return fleet.ActivityPage{}, fmt.Errorf("read activity: %w", driverError{})
}

func (f failingFleet) History(context.Context, string, fleet.HistoryQuery) (fleet.HistoryPage, error) {
	return fleet.HistoryPage{}, f.history
}

func (f failingFleet) Charts(context.Context, string, time.Time, time.Time) (fleet.ChartHistory, error) {
	return fleet.ChartHistory{}, f.history
}

func TestFailedRequestsLogAClassifiedCause(t *testing.T) {
	var events bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&events, nil))
	viewer := viewerAuth{
		user:    config.User{Subject: "engineer", Scopes: []config.Scope{{ConnectionID: "one", Namespaces: []string{"team"}}}},
		allowed: true,
	}
	snapshot := fleet.Snapshot{Deployments: []fleet.Deployment{{ID: "private-deployment", ConnectionID: "one", Namespace: "team"}}}
	for _, test := range []struct {
		name, path, route, cause string
		fleet                    Fleet
	}{
		{"snapshot", "/api/v1/snapshot", "/api/v1/snapshot", "database error 28P01", failingFleet{}},
		{"activity", "/api/v1/activity", "/api/v1/activity", "database error 28P01", failingFleet{}},
		{
			"history",
			"/api/v1/deployments/private-deployment/history",
			"/api/v1/deployments/{id}/history",
			"timed out",
			failingFleet{snapshot: snapshot, history: context.DeadlineExceeded},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			events.Reset()
			handler := (&Server{Fleet: test.fleet, Auth: viewer, Logger: logger}).Handler()
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "secret-user") {
				t.Fatalf("failure response = %d %s", response.Code, response.Body.String())
			}
			var event map[string]any
			if err := json.Unmarshal(events.Bytes(), &event); err != nil {
				t.Fatalf("failure was not logged: %q", events.String())
			}
			if event["method"] != "GET" || event["route"] != test.route || event["cause"] != test.cause || event["status"] != float64(503) {
				t.Fatalf("logged event = %v", event)
			}
			if strings.Contains(events.String(), "secret-user") || strings.Contains(events.String(), "private-deployment") {
				t.Fatal("log contained a driver message or path value")
			}
		})
	}
	events.Reset()
	handler := (&Server{Fleet: &memoryFleet{}, Auth: viewer, Logger: logger}).Handler()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/deployments/missing/history", nil))
	if events.Len() != 0 {
		t.Fatal("client errors were logged as failures")
	}
	if failureCause(errors.New("raw provider text")) != "unclassified" {
		t.Fatal("an unknown error was not classified")
	}
}
