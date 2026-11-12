package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

type configurationRepository struct {
	credentials map[string]settings.CredentialRecord
	mu          sync.Mutex
	record      settings.Record
	changes     []settings.Change
}

func TestReaderAccessBeforeCredentialsIsScopedAndAdministratorOnly(t *testing.T) {
	service := configurationService(t)
	connection := contract.ManagedConnection{
		Id:             "remote",
		Name:           "Remote",
		Authentication: "remote",
		Namespaces:     []contract.ManagedNamespace{{Name: "team", Environment: "production"}},
		Operators: []contract.ManagedOperator{
			{Id: "operator", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"team"}},
		},
	}
	payload, err := json.Marshal(contract.ReaderAccessInput{Connection: connection})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		auth   viewerAuth
		origin string
		code   int
	}{
		{"administrator", viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true}, "https://console.example", 200},
		{"reader", viewerAuth{user: config.User{Subject: "reader"}, allowed: true}, "https://console.example", 403},
		{"anonymous", viewerAuth{}, "https://console.example", 401},
		{"cross origin", viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true}, "https://other.example", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := (&Server{
				Fleet:           &memoryFleet{},
				Auth:            test.auth,
				Settings:        service,
				Origin:          "https://console.example",
				ReaderName:      "chart-reader",
				ReaderNamespace: "console",
			}).Handler()
			request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/connections/rbac", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", test.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("status = %d: %s", response.Code, response.Body.String())
			}
			if test.code == 200 {
				var access contract.ReaderAccessResult
				if err := json.Unmarshal(response.Body.Bytes(), &access); err != nil {
					t.Fatal(err)
				}
				if access.ServiceAccount != "console-reader" || access.ServiceAccountNamespace != "console-system" ||
					!strings.Contains(access.Rbac, "resourceNames:\n  - controller") ||
					strings.Contains(access.Rbac, "ClusterRole") ||
					strings.Contains(access.Rbac, "secrets") {
					t.Fatal("remote reader RBAC escaped namespace or named controller scope")
				}
			}
		})
	}
	_, revision := service.Current()
	if revision != 1 {
		t.Fatal("generating reader instructions changed configuration")
	}
	connection.Authentication = "local"
	payload, err = json.Marshal(contract.ReaderAccessInput{Connection: connection})
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{
		Fleet:           &memoryFleet{},
		Auth:            viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true},
		Settings:        service,
		Origin:          "https://console.example",
		ReaderName:      "chart-reader",
		ReaderNamespace: "console",
	}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/connections/rbac", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://console.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var access contract.ReaderAccessResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &access) != nil || access.ServiceAccount != "chart-reader" ||
		access.ServiceAccountNamespace != "console" {
		t.Fatal("hosting cluster instructions did not use the configured console ServiceAccount")
	}
}

// previewCluster serves two OpenBaoClusters to the private reader token: one with a
// 14-hour-old backup and one created five minutes ago. Optional APIs are forbidden.
func previewCluster(t *testing.T, now time.Time) *httptest.Server {
	t.Helper()
	resources := fmt.Sprintf(
		`{
			"apiVersion": "openbao.org/v1alpha1",
			"kind": "OpenBaoClusterList",
			"metadata": {"resourceVersion":"100"},
			"items": [
				{
					"apiVersion": "openbao.org/v1alpha1",
					"kind": "OpenBaoCluster",
					"metadata": {"name": "bao", "namespace": "team", "uid": "bao-uid", "generation":1},
					"spec": {"replicas": 3, "backup": {}, "unapprovedPrivateField": "private-resource-field"},
					"status": {"observedGeneration":1,"readyReplicas": 3, "backup": {"consecutiveFailures":0,"lastBackupTime": "%s"}}
				},
				{
					"apiVersion": "openbao.org/v1alpha1",
					"kind": "OpenBaoCluster",
					"metadata": {"name": "starting", "namespace": "team", "uid": "starting-uid", "generation":1, "creationTimestamp": "%s"},
					"spec": {"replicas": 3},
					"status": {"observedGeneration":1,"readyReplicas": 0}
				}
			]
		}`,
		now.Add(-14*time.Hour).Format(time.RFC3339),
		now.Add(-5*time.Minute).Format(time.RFC3339),
	)
	cluster := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer private-reader-token" {
			t.Error("check used a mutation or the wrong credential")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/apis/openbao.org/v1alpha1/namespaces/team/openbaoclusters":
			_, _ = w.Write([]byte(resources))
		case "/apis/openbao.org/v1alpha1/namespaces/team/openbaorestores", "/apis/openbao.org/v1alpha1/namespaces/team/openbaorecoveries":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","reason":"Forbidden",` +
				`"message":"private-server-message","code":403}`))
		default:
			t.Errorf("check escaped enrolled resource paths: %s", request.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(cluster.Close)
	return cluster
}

func TestConnectionCheckProjectsPreviewWithoutPersistingOrExposingCredentials(t *testing.T) {
	service := configurationService(t)
	configured, baseRevision := service.Current()
	configured.MaxBackupAgeHours, configured.StartupGraceMinutes = 12, 10
	if _, err := service.Save(context.Background(), baseRevision, configured, "admin", "Configure preview policy"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	cluster := previewCluster(t, now)
	credential := config.Credential{
		Server: cluster.URL,
		CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cluster.Certificate().Raw})),
		Token:  "private-reader-token",
	}
	payload, err := json.Marshal(contract.ConnectionCheck{
		Connection: contract.ManagedConnection{
			Id:             "preview",
			Name:           "Preview",
			Authentication: "remote",
			Namespaces:     []contract.ManagedNamespace{{Name: "team", Environment: "other"}},
		},
		Credential: &contract.ClusterCredential{Server: credential.Server, Ca: credential.CA, Token: credential.Token},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, body := postConnectionCheck(t, service, payload)
	t.Run("inventory and optional APIs", func(t *testing.T) {
		if len(result.Deployments) != 2 || result.Deployments[0].Name != "bao" || result.Deployments[0].Recovery != nil ||
			result.Diagnostics.State != "available" || result.Connection.Capabilities[1].State != "denied" {
			t.Fatal("preview lost workload inventory or hid optional API limitations")
		}
	})
	t.Run("saved policy", func(t *testing.T) {
		if bao := result.Deployments[0]; bao.Health != "attention" || len(bao.Issues) != 1 || bao.Issues[0].Rule != "backup-overdue" {
			t.Fatal("preview did not apply the configured 12-hour backup threshold to a 14-hour-old backup")
		}
		if starting := result.Deployments[1]; starting.Health != "unknown" || len(starting.Issues) != 0 {
			t.Fatal("preview ignored the configured startup grace for a newly created deployment")
		}
	})
	t.Run("matches collection", func(t *testing.T) {
		connection := config.Connection{
			ID:         "preview",
			Name:       "Preview",
			Namespaces: []config.Namespace{{Name: "team", Environment: "other"}},
			Access:     config.Access{Mode: config.AccessRemote, Credential: &credential},
		}
		assertMatchesCollection(t, connection, configured.Policy(), now, result.Deployments)
	})
	t.Run("no private data", func(t *testing.T) {
		for _, private := range []string{"private-reader-token", "private-resource-field", "private-server-message"} {
			if strings.Contains(body, private) {
				t.Fatal("check exposed private input or unapproved Kubernetes data")
			}
		}
	})
	t.Run("nothing persisted", func(t *testing.T) {
		current, revision := service.Current()
		if revision != 2 || len(current.Connections) != 1 || current.Connections[0].ID != "cluster" {
			t.Fatal("checking a connection changed persisted enrollment")
		}
	})
}

// postConnectionCheck runs a connection check as an administrator and returns the decoded
// result and the raw response body.
func postConnectionCheck(t *testing.T, service *settings.Service, payload []byte) (contract.ConnectionCheckResult, string) {
	t.Helper()
	handler := (&Server{
		Fleet:    &memoryFleet{},
		Auth:     viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true},
		Settings: service,
		Origin:   "https://console.example",
	}).Handler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/connections/check", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://console.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("check = %d: %s", response.Code, response.Body.String())
	}
	var result contract.ConnectionCheckResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result, response.Body.String()
}

// assertMatchesCollection requires a preview to report the same health and findings as
// a collection under the same policy.
func assertMatchesCollection(
	t *testing.T,
	connection config.Connection,
	policy fleet.Policy,
	now time.Time,
	preview []contract.Deployment,
) {
	t.Helper()
	reader, err := collector.New(connection, policy)
	if err != nil {
		t.Fatal(err)
	}
	collected := mapSlice(reader.Collect(context.Background(), now).Deployments, deploymentContract)
	if len(collected) != len(preview) {
		t.Fatal("preview and collection returned different workload inventories")
	}
	for i, deployment := range collected {
		if issueRules(deployment) != issueRules(preview[i]) || deployment.Health != preview[i].Health {
			t.Fatal("preview evidence differs from collection under the saved policy")
		}
	}
}

func issueRules(deployment contract.Deployment) string {
	rules := []string{}
	for _, issue := range deployment.Issues {
		rules = append(rules, issue.Rule)
	}
	return strings.Join(rules, ",")
}

func (r *configurationRepository) LoadSettings(context.Context) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.record.Revision == 0 {
		return settings.Record{}, settings.ErrMissing
	}
	return r.record, nil
}

func (r *configurationRepository) SaveSettings(
	_ context.Context,
	revision int64,
	payload []byte,
	actor, summary string,
	writes []settings.CredentialRecord,
	keep []string,
) (settings.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision {
		return settings.Record{}, settings.ErrConflict
	}
	if r.credentials == nil {
		r.credentials = map[string]settings.CredentialRecord{}
	}
	for _, row := range writes {
		if row.Revision != r.credentials[row.ConnectionID].Revision+1 {
			return settings.Record{}, settings.ErrConflict
		}
	}
	for _, row := range writes {
		r.credentials[row.ConnectionID] = row
	}
	retained := map[string]bool{}
	for _, id := range keep {
		retained[id] = true
	}
	for id := range r.credentials {
		if !retained[id] {
			delete(r.credentials, id)
		}
	}

	r.record = settings.Record{Revision: revision + 1, Payload: append([]byte{}, payload...)}
	r.changes = append(r.changes, settings.Change{Revision: revision + 1, Actor: actor, Summary: summary, At: time.Now()})
	return r.record, nil
}

func (r *configurationRepository) SettingsHistory(context.Context) ([]settings.Change, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]settings.Change{}, r.changes...), nil
}

func (*configurationRepository) RetainedConnectionIDs(context.Context, int) ([]string, error) {
	return nil, nil
}

func configurationService(t *testing.T) *settings.Service {
	t.Helper()
	return configurationServiceFor(t, "https://identity.example")
}

// configurationServiceFor binds the test access rules to issuer.
func configurationServiceFor(t *testing.T, issuer string) *settings.Service {
	t.Helper()
	seed := config.Config{
		MaxBackupAgeHours: 24,
		Users:             []config.User{{Subject: "admin", Admin: true}, {Subject: "reader"}},
		Connections: []config.Connection{
			{
				ID:         "cluster",
				Name:       "Cluster",
				Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
				Access: config.Access{
					Mode:       config.AccessRemote,
					Credential: &config.Credential{Server: "https://api.example", Token: "private-reader-token"},
				},
			},
		},
	}
	seed.SeedIdentity(config.Identity{Issuer: issuer, ClientID: "console", ClientSecret: "private-client-secret"})
	service, err := settings.Open(context.Background(), &configurationRepository{}, bytes.Repeat([]byte{5}, 32), &seed)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestSettingsAuthorizationRedactionConflictAndCredentialPreservation(t *testing.T) {
	service := configurationService(t)
	handler := (&Server{
		Fleet:    &memoryFleet{},
		Auth:     viewerAuth{user: config.User{Subject: "admin", Admin: true}, allowed: true},
		Settings: service,
		Origin:   "https://console.example",
	}).Handler()
	request := func(method, path, body, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	response := request("GET", "/api/v1/settings", "", "")
	if response.Code != 200 || strings.Contains(response.Body.String(), "private-client-secret") ||
		strings.Contains(response.Body.String(), "private-reader-token") {
		t.Fatal("settings leaked credentials or could not be read")
	}
	var view contract.Settings
	if err := json.Unmarshal(response.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	view.Configuration.Connections[0].Name = "Changed name"
	payload, _ := json.Marshal(
		contract.SettingsUpdate{Revision: view.Revision, Configuration: view.Configuration, Credentials: []contract.CredentialUpdate{}},
	)
	if got := request("PUT", "/api/v1/settings", string(payload), "https://other.example"); got.Code != 403 {
		t.Fatal("cross-origin update accepted")
	}
	if got := request("PUT", "/api/v1/settings", string(payload), "https://console.example"); got.Code != 200 {
		t.Fatalf("save = %d: %s", got.Code, got.Body.String())
	}
	current, revision := service.Current()
	if current.Connections[0].Access.Credential.Token != "private-reader-token" || revision != 2 {
		t.Fatal("saving public metadata changed the credential")
	}
	if current.AccessIssuer != "https://identity.example" {
		t.Fatal("saving access rules changed their issuer")
	}
	if got := request("PUT", "/api/v1/settings", string(payload), "https://console.example"); got.Code != 409 {
		t.Fatal("stale save accepted")
	}
	payload, _ = json.Marshal(contract.SettingsUpdate{Revision: revision, Configuration: configurationView(current)})
	if got := request(
		"PUT",
		"/api/v1/settings",
		strings.TrimSuffix(string(payload), "}")+`,"unexpected":true}`,
		"https://console.example",
	); got.Code != 400 {
		t.Fatal("unknown request fields accepted")
	}
	current.Users = []config.User{{Subject: "reader"}}
	payload, _ = json.Marshal(contract.SettingsUpdate{Revision: revision, Configuration: configurationView(current)})
	if got := request("PUT", "/api/v1/settings", string(payload), "https://console.example"); got.Code != 400 {
		t.Fatal("last administrator removed")
	}
	for _, principal := range []viewerAuth{{allowed: false}, {user: config.User{Subject: "reader"}, allowed: true}} {
		denied := (&Server{Fleet: &memoryFleet{}, Auth: principal, Settings: service}).Handler()
		response := httptest.NewRecorder()
		denied.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
		if response.Code != 401 && response.Code != 403 {
			t.Fatal("settings visible without administrator access")
		}
	}
}

func (r *configurationRepository) LoadCredentials(context.Context) ([]settings.CredentialRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := []settings.CredentialRecord{}
	for _, row := range r.credentials {
		rows = append(rows, row)
	}
	return rows, nil
}

func (r *configurationRepository) SaveCredential(_ context.Context, revision int64, row settings.CredentialRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if revision != r.record.Revision || row.Revision != r.credentials[row.ConnectionID].Revision+1 {
		return settings.ErrConflict
	}
	r.credentials[row.ConnectionID] = row
	return nil
}
