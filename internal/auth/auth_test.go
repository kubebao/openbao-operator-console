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
	enabled := true
	a := NewPending(func(context.Context, string, []string) (config.User, bool) {
		return config.User{Subject: "reader", Admin: enabled}, enabled
	})
	a.logins.sessions["session"] = session{subject: "reader", expires: time.Now().Add(time.Hour)}
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

func TestProviderReplacementEndsSessionsAndPendingLogins(t *testing.T) {
	a := NewPending(func(_ context.Context, subject string, _ []string) (config.User, bool) {
		return config.User{Subject: subject}, true
	})
	state, err := a.logins.begin(transaction{returnTo: "/"})
	if err != nil {
		t.Fatal(err)
	}
	before := a.logins.current()
	id, err := a.logins.open(before, session{subject: "reader", expires: time.Now().Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	a.ReplaceProvider(&Provider{})
	if _, ok := a.logins.lookup(id); ok {
		t.Fatal("session survived provider replacement")
	}
	if _, ok := a.logins.take(state); ok {
		t.Fatal("login started before replacement remained valid")
	}
	if _, err := a.logins.open(before, session{subject: "reader", expires: time.Now().Add(time.Hour)}); err == nil {
		t.Fatal("login from the previous generation opened a session")
	}
	if a.activeProvider() == nil {
		t.Fatal("replacement did not activate the provider")
	}
}
