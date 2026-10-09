package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/net/html"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	consolesettings "github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

// This optional integration test exercises the real reader, database, OIDC
// session, authorization, and HTTP contract together. The identity provider is
// a signed TLS test authority; it does not qualify a vendor identity provider.
func TestKindPostgresAndOIDCSession(t *testing.T) {
	testLiveSession(t, false)
}

func TestKindKeycloakSession(t *testing.T) {
	testLiveSession(t, true)
}

type keycloakSettings struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Subject      string `json:"subject"`
	Username     string `json:"username"`
	Password     string `json:"password"`
}

func testLiveSession(t *testing.T, keycloak bool) {
	t.Helper()
	var settings keycloakSettings
	var ca []byte
	if keycloak {
		path := os.Getenv("CONSOLE_TEST_KEYCLOAK_CONFIG")
		if path == "" {
			t.Skip("requires isolated Keycloak development credentials")
		}
		data, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(data, &settings) != nil {
			t.Fatal("cannot read Keycloak test settings")
		}
		ca, err = os.ReadFile(filepath.Join(filepath.Dir(path), "server.crt"))
		if err != nil {
			t.Fatal("cannot read Keycloak test CA")
		}
	}
	dsn, kubeconfig := os.Getenv("CONSOLE_TEST_DATABASE_URL"), os.Getenv("CONSOLE_TEST_KUBECONFIG")
	if dsn == "" || kubeconfig == "" {
		t.Skip("requires isolated console_test PostgreSQL and console-dev reader credential")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Path != "/console_test" {
		t.Fatal("live qualification requires console_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id := fmt.Sprintf("qualification-%d", time.Now().UnixNano())
	connection := config.Connection{ID: id, Name: "Qualification", Kubeconfig: kubeconfig, Context: "kind-console-dev",
		Namespaces: []config.Namespace{{Name: "console-sample", Environment: "development"}},
		Operators:  []config.Operator{{ID: "operator", Namespace: "console-system", Deployment: "console-operator-openbao-operator-controller", Container: "manager", Namespaces: []string{"console-sample"}}}}
	reader, err := collector.New(connection, evidence.Policy{MaxBackupAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	batch := reader.Collect(ctx, time.Now().UTC())
	if batch.Connection.Collection != "current" || len(batch.Deployments) == 0 || len(batch.Connection.Operators) != 1 || batch.Connection.Operators[0].Collection != "current" {
		t.Fatal("live reader failed qualification")
	}
	repository, err := store.Open(ctx, dsn, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	var providerContext context.Context
	if keycloak {
		providerContext, err = auth.ProviderContext(ctx, filepath.Join(filepath.Dir(os.Getenv("CONSOLE_TEST_KEYCLOAK_CONFIG")), "server.crt"))
		if err != nil {
			t.Fatal("cannot configure Keycloak certificate trust")
		}
	} else {
		provider := signedTestProvider(t)
		defer provider.Close()
		providerContext = oidc.ClientContext(ctx, provider.Client())
		settings = keycloakSettings{Issuer: provider.URL, ClientID: "console", ClientSecret: "test-secret", Subject: "engineer"}
	}
	mux := http.NewServeMux()
	application := httptest.NewUnstartedServer(mux)
	if keycloak {
		_ = application.Listener.Close()
		application.Listener, err = net.Listen("tcp", "127.0.0.1:8554")
		if err != nil {
			t.Fatal("Keycloak test callback port 8554 is unavailable")
		}
	}
	application.Config.BaseContext = func(net.Listener) context.Context { return providerContext }
	application.StartTLS()
	defer application.Close()
	user := config.User{Subject: settings.Subject, Name: "Engineer", Admin: true, Scopes: []config.Scope{{ConnectionID: id, Namespaces: []string{"console-sample"}}}}
	authentication, err := auth.NewOIDC(providerContext, settings.Issuer, settings.ClientID, settings.ClientSecret, application.URL+"/auth/callback", []config.User{user})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := consolesettings.New(ctx, &configurationRepository{}, bytes.Repeat([]byte{4}, 32), config.Config{Connections: []config.Connection{connection}, Users: []config.User{user}, MaxBackupAgeHours: 24, Identity: &config.Identity{Issuer: settings.Issuer, ClientID: settings.ClientID, ClientSecret: settings.ClientSecret}})
	if err != nil {
		t.Fatal(err)
	}
	authentication.SetResolver(configuration.Resolve)
	mux.Handle("/", (&Server{Repository: repository, Auth: authentication, Settings: configuration, Origin: application.URL}).Handler())

	client := application.Client()
	if keycloak && !client.Transport.(*http.Transport).TLSClientConfig.RootCAs.AppendCertsFromPEM(ca) {
		t.Fatal("cannot trust Keycloak test certificate")
	}
	response, err := client.Get(application.URL + "/api/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("live evidence was accessible without authentication")
	}
	client.Jar, _ = cookiejar.New(nil)
	response, err = client.Get(application.URL + "/auth/login")
	if err != nil {
		t.Fatal(err)
	}
	if keycloak {
		loginAction := keycloakLoginAction(t, response.Body)
		_ = response.Body.Close()
		response, err = client.PostForm(loginAction, url.Values{"username": {settings.Username}, "password": {settings.Password}})
		if err != nil {
			t.Fatal("Keycloak login request failed")
		}
	}
	_ = response.Body.Close()
	read := func() contract.Snapshot {
		t.Helper()
		response, err := client.Get(application.URL + "/api/v1/snapshot")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var snapshot contract.Snapshot
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&snapshot) != nil {
			t.Fatal("authenticated live snapshot failed")
		}
		return snapshot
	}
	snapshot := read()
	if snapshot.Mode != "live" || snapshot.Viewer.Subject != settings.Subject || len(snapshot.Connections) != 1 || len(snapshot.Deployments) != len(batch.Deployments) || snapshot.Deployments[0].OperatorId == nil {
		t.Fatal("authenticated snapshot lost live identity or leaked other database enrollments")
	}
	version := snapshot.Connections[0].Operators[0].Version
	batch.Connection.Operators[0].Collection = "denied"
	batch.Connection.Operators[0].Version = nil
	batch.Connection.Operators[0].LastSuccessAt = nil
	if err := repository.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	snapshot = read()
	operator := snapshot.Connections[0].Operators[0]
	if operator.Collection != "denied" || operator.LastSuccessAt == nil || (version != nil && (operator.Version == nil || *operator.Version != *version)) || snapshot.Connections[0].Collection != "current" {
		t.Fatal("operator read failure concealed last-known facts or stopped workload inventory")
	}
	// The onboarding API tests the real enrolled reader without Kubernetes mutations.
	current, revision := configuration.Current()
	checkPayload, _ := json.Marshal(contract.ConnectionCheck{Connection: connectionView(current.Connections[0])})
	post, _ := http.NewRequest("POST", application.URL+"/api/v1/settings/connections/check", bytes.NewReader(checkPayload))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Origin", application.URL)
	response, err = client.Do(post)
	if err != nil {
		t.Fatal(err)
	}
	var checkResult contract.ConnectionCheckResult
	if response.StatusCode != 200 || json.NewDecoder(response.Body).Decode(&checkResult) != nil || checkResult.Connection.Collection != "current" || len(checkResult.Connection.Operators) != 1 {
		t.Fatal("real onboarding access check failed")
	}
	_ = response.Body.Close()
	current.Connections[0].Disabled = true
	updatePayload, _ := json.Marshal(contract.SettingsUpdate{Revision: revision, Configuration: configurationView(current), Credentials: []contract.CredentialUpdate{}})
	put, _ := http.NewRequest("PUT", application.URL+"/api/v1/settings", bytes.NewReader(updatePayload))
	put.Header.Set("Content-Type", "application/json")
	put.Header.Set("Origin", application.URL)
	response, err = client.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal("real session could not save configuration")
	}
	snapshot = read()
	if snapshot.Connections[0].Collection != "stale" || len(snapshot.Deployments) != len(batch.Deployments) {
		t.Fatal("pause concealed inventory or claimed current collection")
	}
	logout, _ := http.NewRequest(http.MethodPost, application.URL+"/auth/logout", nil)
	logout.Header.Set("Origin", application.URL)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err = client.Do(logout)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	response, err = client.Get(application.URL + "/api/v1/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("logout retained live access")
	}
}

func keycloakLoginAction(t *testing.T, body io.Reader) string {
	t.Helper()
	document, err := html.Parse(io.LimitReader(body, 1<<20))
	if err != nil {
		t.Fatal("cannot parse Keycloak login form")
	}
	var action string
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "form" {
			attributes := map[string]string{}
			for _, attribute := range node.Attr {
				attributes[attribute.Key] = attribute.Val
			}
			if attributes["id"] == "kc-form-login" {
				action = attributes["action"]
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	endpoint, err := url.Parse(action)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host != "localhost:8543" {
		t.Fatal("expected the isolated Keycloak HTTPS login form")
	}
	return action
}

func signedTestProvider(t *testing.T) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "qualification"))
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce, challenge string
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "qualification", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("GET /authorize", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		nonce, challenge = r.URL.Query().Get("nonce"), r.URL.Query().Get("code_challenge")
		mu.Unlock()
		callback, err := url.Parse(r.URL.Query().Get("redirect_uri"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		query := callback.Query()
		query.Set("code", "qualification")
		query.Set("state", r.URL.Query().Get("state"))
		callback.RawQuery = query.Encode()
		http.Redirect(w, r, callback.String(), http.StatusFound)
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		mu.Lock()
		defer mu.Unlock()
		if nonce == "" || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token, err := jwt.Signed(signer).Claims(map[string]interface{}{"iss": issuer, "aud": "console", "sub": "engineer", "nonce": nonce, "exp": time.Now().Add(time.Hour).Unix()}).Serialize()
		nonce = ""
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "test-provider-token", "token_type": "Bearer", "id_token": token})
	})
	server := httptest.NewTLSServer(mux)
	issuer = server.URL
	return server
}
