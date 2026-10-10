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

func TestConnectionCheckProjectsPreviewWithoutPersistingOrExposingCredentials(t *testing.T) {
	service := configurationService(t)
	configured, baseRevision := service.Current()
	configured.MaxBackupAgeHours, configured.StartupGraceMinutes = 12, 10
	if _, err := service.Save(context.Background(), baseRevision, configured, "admin", "Configure preview policy"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resources := fmt.Sprintf(
		`{
			"apiVersion": "openbao.org/v1alpha1",
			"kind": "OpenBaoClusterList",
			"metadata": {},
			"items": [
				{
					"apiVersion": "openbao.org/v1alpha1",
					"kind": "OpenBaoCluster",
					"metadata": {"name": "bao", "namespace": "team", "uid": "bao-uid"},
					"spec": {"replicas": 3, "backup": {}, "unapprovedPrivateField": "private-resource-field"},
					"status": {"readyReplicas": 3, "backup": {"lastBackupTime": "%s"}}
				},
				{
					"apiVersion": "openbao.org/v1alpha1",
					"kind": "OpenBaoCluster",
					"metadata": {"name": "starting", "namespace": "team", "uid": "starting-uid", "creationTimestamp": "%s"},
					"spec": {"replicas": 3},
					"status": {"readyReplicas": 0}
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
	defer cluster.Close()
	connection := contract.ManagedConnection{
		Id:             "preview",
		Name:           "Preview",
		Authentication: "remote",
		Namespaces:     []contract.ManagedNamespace{{Name: "team", Environment: "other"}},
	}
	payload, err := json.Marshal(
		contract.ConnectionCheck{
			Connection: connection,
			Credential: &contract.ClusterCredential{
				Server: cluster.URL,
				Ca:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cluster.Certificate().Raw})),
				Token:  "private-reader-token",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
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
	if response.Code != 200 {
		t.Fatalf("check = %d: %s", response.Code, response.Body.String())
	}
	var result contract.ConnectionCheckResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Deployments) != 2 || result.Deployments[0].Name != "bao" || result.Deployments[0].Recovery != nil ||
		result.Diagnostics.State != "available" ||
		result.Connection.Capabilities[1].State != "denied" {
		t.Fatal("preview lost workload inventory or hid optional API limitations")
	}
	if result.Deployments[0].Health != "attention" || len(result.Deployments[0].Issues) != 1 ||
		result.Deployments[0].Issues[0].Rule != "backup-overdue" {
		t.Fatal("preview did not apply the configured 12-hour backup threshold to a 14-hour-old backup")
	}
	if result.Deployments[1].Health != "unknown" || len(result.Deployments[1].Issues) != 0 {
		t.Fatal("preview ignored the configured startup grace for a newly created deployment")
	}
	reader, err := collector.New(
		config.Connection{
			ID:         "preview",
			Name:       "Preview",
			Namespaces: []config.Namespace{{Name: "team", Environment: "other"}},
			Access: config.Access{Mode: config.AccessRemote, Credential: &config.Credential{
				Server: cluster.URL,
				CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cluster.Certificate().Raw})),
				Token:  "private-reader-token",
			}},
		},
		configured.Policy(),
	)
	if err != nil {
		t.Fatal(err)
	}
	managed := reader.Collect(context.Background(), now)
	if len(managed.Deployments) != len(result.Deployments) {
		t.Fatal("preview and collection returned different workload inventories")
	}
	for i, collected := range managed.Deployments {
		deployment, preview := deploymentContract(collected), result.Deployments[i]
		if deployment.Health != preview.Health || len(deployment.Issues) != len(preview.Issues) {
			t.Fatal("preview health differs from collection under the saved policy")
		}
		for j, issue := range deployment.Issues {
			if issue.Rule != preview.Issues[j].Rule {
				t.Fatal("preview finding differs from collection under the saved policy")
			}
		}
	}
	for _, private := range []string{"private-reader-token", "private-resource-field", "private-server-message"} {
		if strings.Contains(response.Body.String(), private) {
			t.Fatal("check exposed private input or unapproved Kubernetes data")
		}
	}
	current, revision := service.Current()
	if revision != 2 || len(current.Connections) != 1 || current.Connections[0].ID != "cluster" {
		t.Fatal("checking a connection changed persisted enrollment")
	}
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

func configurationService(t *testing.T) *settings.Service {
	t.Helper()
	service, err := settings.New(
		context.Background(),
		&configurationRepository{},
		bytes.Repeat([]byte{5}, 32),
		config.Config{
			MaxBackupAgeHours: 24,
			Users:             []config.User{{Subject: "admin", Admin: true}, {Subject: "reader"}},
			Identity: &config.Identity{
				Issuer:       "https://identity.example",
				ClientID:     "console",
				ClientSecret: "private-client-secret",
			},
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
		},
	)
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

func TestIdentityTrialCommitsOnlyAfterVerifiedAdministratorLogin(t *testing.T) {
	provider := signedTestProvider(t)
	defer provider.Close()
	ctx := context.Background()
	service := configurationService(t)
	current, revision := service.Current()
	current.Users = []config.User{{Subject: "engineer", Admin: true}}
	if _, err := service.Save(ctx, revision, current, "bootstrap", "Enroll test administrator"); err != nil {
		t.Fatal(err)
	}
	authentication := testAuthenticator(t, provider, service)
	handler := (&Server{
		Fleet:    &memoryFleet{},
		Auth:     authentication,
		Settings: service,
		Origin:   "https://console.example",
	}).Handler()
	login := httptest.NewRecorder()
	authentication.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
	// Complete the signed test provider login through its HTTPS endpoints.
	sessionCookie := completeTestLogin(t, provider, authentication, login.Header().Get("Location"), login.Result().Cookies())
	current, revision = service.Current()
	ca := testProviderCA(provider)
	payload, _ := json.Marshal(
		contract.IdentityTrial{
			Revision: revision,
			Identity: contract.IdentityInput{
				Issuer:       provider.URL,
				ClientId:     "console",
				Ca:           ca,
				GroupsClaim:  "groups",
				ClientSecret: new("candidate-secret"),
			},
		},
	)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/identity/trial", bytes.NewReader(payload))
	request.AddCookie(sessionCookie)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("Content-Type", "application/json")
	trial := httptest.NewRecorder()
	handler.ServeHTTP(trial, request)
	if trial.Code != 200 {
		t.Fatalf("trial failed: %d %s", trial.Code, trial.Body.String())
	}
	before, _ := service.Current()
	if before.Identity.Issuer != current.Identity.Issuer {
		t.Fatal("identity activated before login")
	}
	var redirect contract.LoginRedirect
	if err := json.Unmarshal(trial.Body.Bytes(), &redirect); err != nil {
		t.Fatal(err)
	}
	completeTestLogin(t, provider, authentication, redirect.Url, trial.Result().Cookies())
	after, afterRevision := service.Current()
	if after.Identity.Issuer != provider.URL || afterRevision != revision+1 {
		t.Fatal("verified trial did not activate")
	}
	previousRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	previousRequest.AddCookie(sessionCookie)
	if _, allowed := authentication.Viewer(previousRequest); allowed {
		t.Fatal("provider activation retained old sessions")
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
