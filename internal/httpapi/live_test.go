package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
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
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	consolesettings "github.com/kubebao/openbao-operator-console/internal/settings"
	"github.com/kubebao/openbao-operator-console/internal/store"
	"github.com/kubebao/openbao-operator-console/internal/view"
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

func qualificationConnection(id, kubeconfig string) config.Connection {
	return config.Connection{
		ID:         id,
		Name:       "Qualification",
		Namespaces: []config.Namespace{{Name: "console-sample", Environment: "development"}},
		Operators: []config.Operator{
			{
				ID:         "operator",
				Namespace:  "console-system",
				Deployment: "console-operator-openbao-operator-controller",
				Container:  "manager",
				Namespaces: []string{"console-sample"},
			},
		},
		Access: config.Access{Mode: config.AccessKubeconfig, Kubeconfig: kubeconfig, Context: "kind-console-dev"},
	}
}

// liveIdentity returns the isolated Keycloak or a signed test authority, the provider
// transport context, and an extra CA for the browser.
func liveIdentity(t *testing.T, ctx context.Context, keycloak bool) (keycloakSettings, context.Context, []byte) {
	t.Helper()
	if !keycloak {
		provider := signedTestProvider(t)
		t.Cleanup(provider.Close)
		identity := keycloakSettings{Issuer: provider.URL, ClientID: "console", ClientSecret: "test-secret", Subject: "engineer"}
		return identity, oidc.ClientContext(ctx, provider.Client()), nil
	}
	path := os.Getenv("CONSOLE_TEST_KEYCLOAK_CONFIG")
	if path == "" {
		t.Skip("requires isolated Keycloak development credentials")
	}
	var identity keycloakSettings
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &identity) != nil {
		t.Fatal("cannot read Keycloak test settings")
	}
	certificate := filepath.Join(filepath.Dir(path), "server.crt")
	ca, err := os.ReadFile(certificate)
	if err != nil {
		t.Fatal("cannot read Keycloak test CA")
	}
	roots, err := x509.SystemCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatal("cannot configure Keycloak certificate trust")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	return identity, oidc.ClientContext(ctx, client), ca
}

// liveSession is a browser session against a console served over TLS.
type liveSession struct {
	t      *testing.T
	client *http.Client
	url    string
}

func (s liveSession) status(method, path string, payload any) int {
	s.t.Helper()
	response := s.send(method, path, payload)
	_ = response.Body.Close()
	return response.StatusCode
}

// send makes a same-origin request with an optional JSON body.
func (s liveSession) send(method, path string, payload any) *http.Response {
	s.t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			s.t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, s.url+path, body)
	if err != nil {
		s.t.Fatal(err)
	}
	request.Header.Set("Origin", s.url)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(request)
	if err != nil {
		s.t.Fatal(err)
	}
	return response
}

func (s liveSession) snapshot() contract.Snapshot {
	s.t.Helper()
	response := s.send(http.MethodGet, "/api/v1/snapshot", nil)
	defer func() { _ = response.Body.Close() }()
	var snapshot contract.Snapshot
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&snapshot) != nil {
		s.t.Fatal("authenticated live snapshot failed")
	}
	return snapshot
}

// signIn completes the authorization code flow through the provider's login form.
func (s liveSession) signIn(keycloak bool, identity keycloakSettings) {
	s.t.Helper()
	s.client.Jar, _ = cookiejar.New(nil)
	response := s.send(http.MethodGet, "/auth/login", nil)
	if keycloak {
		action := keycloakLoginAction(s.t, response.Body)
		_ = response.Body.Close()
		var err error
		response, err = s.client.PostForm(action, url.Values{"username": {identity.Username}, "password": {identity.Password}})
		if err != nil {
			s.t.Fatal("Keycloak login request failed")
		}
	}
	_ = response.Body.Close()
}

// startLiveConsole serves a console for one enrolled administrator. Keycloak requires
// the registered callback port 8554.
func startLiveConsole(
	t *testing.T,
	ctx context.Context,
	keycloak bool,
	identity keycloakSettings,
	providerContext context.Context,
	connection config.Connection,
	repository *store.Postgres,
) (*consolesettings.Service, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	application := httptest.NewUnstartedServer(mux)
	if keycloak {
		_ = application.Listener.Close()
		listener, err := net.Listen("tcp", "127.0.0.1:8554")
		if err != nil {
			t.Fatal("Keycloak test callback port 8554 is unavailable")
		}
		application.Listener = listener
	}
	application.Config.BaseContext = func(net.Listener) context.Context { return providerContext }
	application.StartTLS()
	t.Cleanup(application.Close)
	user := config.User{
		Subject: identity.Subject,
		Name:    "Engineer",
		Admin:   true,
		Scopes:  []config.Scope{{ConnectionID: connection.ID, Namespaces: []string{"console-sample"}}},
	}
	provider := config.Identity{Issuer: identity.Issuer, ClientID: identity.ClientID, ClientSecret: identity.ClientSecret}
	identityProvider, err := auth.NewProvider(
		providerContext,
		provider.Issuer,
		provider.ClientID,
		provider.ClientSecret,
		application.URL+"/auth/callback",
	)
	if err != nil {
		t.Fatal(err)
	}
	seed := config.Config{
		Connections:       []config.Connection{connection},
		Users:             []config.User{user},
		MaxBackupAgeHours: 24,
	}
	seed.SeedIdentity(provider)
	configuration, err := consolesettings.Open(ctx, &configurationRepository{}, bytes.Repeat([]byte{4}, 32), &seed)
	if err != nil {
		t.Fatal(err)
	}
	authentication := auth.New(identityProvider, configuration.Resolve)
	server := &Server{
		Fleet:    view.Live{Observations: repository, Enrollment: configuration},
		Auth:     authentication,
		Settings: configuration,
		Origin:   application.URL,
	}
	mux.Handle("/", server.Handler())
	return configuration, application
}

// qualifyReader collects the enrolled namespace and controller with the real reader.
func qualifyReader(t *testing.T, ctx context.Context, connection config.Connection) store.Batch {
	t.Helper()
	reader, err := collector.New(connection, fleet.Policy{MaxBackupAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	batch := reader.Collect(ctx, time.Now().UTC())
	if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) == 0 ||
		len(batch.Connection.Operators) != 1 || batch.Connection.Operators[0].Collection != fleet.OperatorCollectionCurrent {
		t.Fatal("live reader failed qualification")
	}
	return batch
}

func testLiveSession(t *testing.T, keycloak bool) {
	t.Helper()
	dsn, kubeconfig := os.Getenv("CONSOLE_TEST_DATABASE_URL"), os.Getenv("CONSOLE_TEST_KUBECONFIG")
	if dsn == "" || kubeconfig == "" {
		t.Skip("requires isolated console_test PostgreSQL and console-dev reader credential")
	}
	if parsed, err := url.Parse(dsn); err != nil || parsed.Path != "/console_test" {
		t.Fatal("live qualification requires console_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	identity, providerContext, ca := liveIdentity(t, ctx, keycloak)
	connection := qualificationConnection(fmt.Sprintf("qualification-%d", time.Now().UnixNano()), kubeconfig)
	batch := qualifyReader(t, ctx, connection)
	repository, err := store.Open(ctx, dsn, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err := repository.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	configuration, application := startLiveConsole(t, ctx, keycloak, identity, providerContext, connection, repository)
	session := liveSession{t: t, client: application.Client(), url: application.URL}
	if keycloak && !session.client.Transport.(*http.Transport).TLSClientConfig.RootCAs.AppendCertsFromPEM(ca) {
		t.Fatal("cannot trust Keycloak test certificate")
	}
	if session.status(http.MethodGet, "/api/v1/snapshot", nil) != http.StatusUnauthorized {
		t.Fatal("live evidence was accessible without authentication")
	}
	session.signIn(keycloak, identity)
	snapshot := session.snapshot()
	if snapshot.Mode != "live" || snapshot.Viewer.Subject != identity.Subject || len(snapshot.Connections) != 1 ||
		len(snapshot.Deployments) != len(batch.Deployments) || snapshot.Deployments[0].OperatorId == nil {
		t.Fatal("authenticated snapshot lost live identity or leaked other database enrollments")
	}
	t.Run("operator read failure keeps last-known facts", func(t *testing.T) {
		version := snapshot.Connections[0].Operators[0].Version
		failed := batch
		failed.Connection.Operators = []fleet.Operator{batch.Connection.Operators[0]}
		failed.Connection.Operators[0].Collection = fleet.OperatorCollectionDenied
		failed.Connection.Operators[0].Version, failed.Connection.Operators[0].LastSuccessAt = nil, nil
		if err := repository.Apply(ctx, failed); err != nil {
			t.Fatal(err)
		}
		current := session.snapshot()
		operator := current.Connections[0].Operators[0]
		if operator.Collection != "denied" || operator.LastSuccessAt == nil ||
			(version != nil && (operator.Version == nil || *operator.Version != *version)) ||
			current.Connections[0].Collection != "current" {
			t.Fatal("operator read failure concealed last-known facts or stopped workload inventory")
		}
	})
	t.Run("onboarding check reads the enrolled cluster", func(t *testing.T) {
		current, _ := configuration.Current()
		response := session.send(
			http.MethodPost,
			"/api/v1/settings/connections/check",
			contract.ConnectionCheck{Connection: connectionView(current.Connections[0])},
		)
		defer func() { _ = response.Body.Close() }()
		var result contract.ConnectionCheckResult
		if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil ||
			result.Connection.Collection != "current" || len(result.Connection.Operators) != 1 {
			t.Fatal("real onboarding access check failed")
		}
	})
	t.Run("pause keeps inventory and marks it stale", func(t *testing.T) {
		current, revision := configuration.Current()
		current.Connections[0].Disabled = true
		update := contract.SettingsUpdate{
			Revision:      revision,
			Configuration: configurationView(current),
			Credentials:   []contract.CredentialUpdate{},
		}
		if session.status(http.MethodPut, "/api/v1/settings", update) != http.StatusOK {
			t.Fatal("real session could not save configuration")
		}
		paused := session.snapshot()
		if paused.Connections[0].Collection != "stale" || len(paused.Deployments) != len(batch.Deployments) {
			t.Fatal("pause concealed inventory or claimed current collection")
		}
	})
	t.Run("logout ends access", func(t *testing.T) {
		session.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		session.status(http.MethodPost, "/auth/logout", nil)
		if session.status(http.MethodGet, "/api/v1/snapshot", nil) != http.StatusUnauthorized {
			t.Fatal("logout retained live access")
		}
	})
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
	return signedTestProviderFor(t, "engineer")
}

// signedTestProviderFor signs ID tokens for subject with its own HTTPS issuer.
func signedTestProviderFor(t *testing.T, subject string) *httptest.Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader("kid", "qualification"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce, challenge string
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/authorize",
			"token_endpoint":                        issuer + "/token",
			"jwks_uri":                              issuer + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "qualification", Algorithm: "RS256", Use: "sig"}},
		})
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
		token, err := jwt.Signed(signer).
			Claims(map[string]any{
				"iss":   issuer,
				"aud":   "console",
				"sub":   subject,
				"nonce": nonce,
				"exp":   time.Now().Add(time.Hour).Unix(),
			}).
			Serialize()
		nonce = ""
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-provider-token", "token_type": "Bearer", "id_token": token})
	})
	server := httptest.NewTLSServer(mux)
	issuer = server.URL
	return server
}
