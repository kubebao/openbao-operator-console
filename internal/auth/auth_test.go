package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestScopeRemovesHiddenGraphCountsHistoryAndNamespaces(t *testing.T) {
	snapshot := fleet.Snapshot{
		Deployments: []fleet.Deployment{
			{ID: "visible", ConnectionID: "cluster", Namespace: "team-a"},
			{ID: "hidden", ConnectionID: "cluster", Namespace: "team-b"},
		},
		Connections: []fleet.Connection{
			{
				ID:         "cluster",
				Namespaces: []string{"team-a", "team-b"},
				Operators: []fleet.Operator{
					{ID: "shared", Namespaces: []string{"team-a", "team-b"}},
					{ID: "hidden", Namespaces: []string{"team-b"}},
				},
				Capabilities: []fleet.Capability{{Namespace: "team-a"}, {Namespace: "team-b"}},
			},
		},
		Relationships: []fleet.Relationship{{ID: "edge", SourceID: "visible", TargetID: "hidden"}},
		Activity:      []fleet.Activity{{ID: "secret-event", DeploymentID: "hidden"}},
	}
	user := config.User{Subject: "a", Scopes: []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team-a"}}}}
	result := Scope(snapshot, user)
	if len(result.Deployments) != 1 || len(result.Relationships) != 0 || len(result.Activity) != 0 {
		t.Fatal("hidden resource leaked through a projected view")
	}
	if len(result.Connections) != 1 || len(result.Connections[0].Namespaces) != 1 {
		t.Fatal("hidden namespace leaked")
	}
	if (config.User{}).CanView("cluster", "team-a") {
		t.Fatal("empty scope granted access")
	}
	if len(result.Connections[0].Operators) != 1 || len(result.Connections[0].Operators[0].Namespaces) != 1 ||
		len(result.Connections[0].Capabilities) != 1 {
		t.Fatal("operator or API metadata leaked outside the namespace scope")
	}
}

func TestExistingSessionUsesCurrentAccessRules(t *testing.T) {
	a := &Authenticator{
		sessions: map[string]session{"session": {subject: "reader", expires: time.Now().Add(time.Hour)}},
		users:    map[string]config.User{},
	}
	enabled := true
	a.SetResolver(func(context.Context, string, []string) (config.User, bool) {
		return config.User{Subject: "reader", Admin: enabled}, enabled
	})
	request := httptest.NewRequest(http.MethodGet, "https://console.example/api/v1/settings", nil)
	request.AddCookie(&http.Cookie{Name: "__Host-console-session", Value: "session"})
	if user, ok := a.Viewer(request); !ok || !user.Admin {
		t.Fatal("existing session did not resolve access")
	}
	enabled = false
	if _, ok := a.Viewer(request); ok {
		t.Fatal("existing session retained revoked access")
	}
}
