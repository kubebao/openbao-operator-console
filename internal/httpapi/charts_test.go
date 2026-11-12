package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/demo"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestChartReadAuthorizesBeforeHistoryAndCounts(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	repository := &memoryFleet{
		snapshot: fleet.Snapshot{GeneratedAt: now, Deployments: []fleet.Deployment{{ID: "bao", ConnectionID: "one", Namespace: "team"}}},
	}
	denied := (&Server{Fleet: repository, Auth: viewerAuth{user: config.User{Subject: "reader"}, allowed: true}}).Handler()
	for _, id := range []string{"bao", "missing"} {
		response := httptest.NewRecorder()
		denied.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/"+id+"/charts", nil))
		if response.Code != http.StatusNotFound || repository.chartReads != 0 || repository.historyReads != 0 {
			t.Fatal("hidden chart resource reached the history reader")
		}
	}
	reader := config.User{Subject: "reader", Scopes: []config.Scope{{ConnectionID: "one", Namespaces: []string{"team"}}}}
	allowed := (&Server{Fleet: repository, Auth: viewerAuth{user: reader, allowed: true}}).Handler()
	data := getPage[contract.ChartHistory](t, allowed, "/api/v1/deployments/bao/charts")
	if repository.chartReads != 1 || repository.historyReads != 0 || !data.End.Equal(now) || data.End.Sub(data.Start) != 7*24*time.Hour {
		t.Fatal("chart read used list pagination or an inconsistent time window")
	}
}

func TestChartReadUsesFixtureHistoryAndClassifiesFailures(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	repository := demo.New(now)
	handler := (&Server{Fleet: repository, Auth: viewerAuth{user: repository.Viewer(), allowed: true}}).Handler()
	data := getPage[contract.ChartHistory](t, handler, "/api/v1/deployments/identity/charts")
	if len(data.Observations) == 0 || !data.End.Equal(now) {
		t.Fatal("fixture chart history is empty")
	}
	failed := failingFleet{
		snapshot: fleet.Snapshot{GeneratedAt: now, Deployments: []fleet.Deployment{{ID: "bao", ConnectionID: "one", Namespace: "team"}}},
		history:  driverError{},
	}
	response := httptest.NewRecorder()
	reader := config.User{Admin: true, Scopes: []config.Scope{{ConnectionID: "one", Namespaces: []string{"team"}}}}
	(&Server{Fleet: failed, Auth: viewerAuth{user: reader, allowed: true}}).Handler().
		ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/deployments/bao/charts", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "secret-user") {
		t.Fatalf("chart failure = %d: %s", response.Code, response.Body.String())
	}
}
