package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/kubebao/openbao-operator-console/internal/auth"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

// trialFixture is a console whose active provider is a signed test authority. The
// administrator "engineer" is signed in; subject "42" and group "platform" are bound to
// the active issuer.
type trialFixture struct {
	active         *httptest.Server
	service        *settings.Service
	authentication *auth.Authenticator
	handler        http.Handler
	session        *http.Cookie
}

func newTrialFixture(t *testing.T) *trialFixture {
	t.Helper()
	active := signedTestProvider(t)
	t.Cleanup(active.Close)
	service := configurationServiceFor(t, active.URL)
	current, revision := service.Current()
	current.Users = []config.User{
		{Subject: "engineer", Name: "Engineer", Admin: true, Scopes: []config.Scope{}},
		{Subject: "42", Name: "Reader", Scopes: []config.Scope{{ConnectionID: "cluster", Namespaces: []string{"team"}}}},
	}
	current.Groups = []config.Group{{Name: "platform", Admin: true, Scopes: []config.Scope{}}}
	if _, err := service.Save(context.Background(), revision, current, "bootstrap", "Enroll test access"); err != nil {
		t.Fatal(err)
	}
	authentication := testAuthenticator(t, active, service)
	handler := (&Server{Fleet: &memoryFleet{}, Auth: authentication, Settings: service, Origin: "https://console.example"}).Handler()
	login := httptest.NewRecorder()
	authentication.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
	session := completeTestLogin(t, active, authentication, login.Header().Get("Location"), login.Result().Cookies())
	return &trialFixture{active: active, service: service, authentication: authentication, handler: handler, session: session}
}

// trial starts an identity trial against candidate with an optional access rules decision.
func (f *trialFixture) trial(
	t *testing.T,
	candidate *httptest.Server,
	rules *contract.IdentityTrialAccessRules,
) *httptest.ResponseRecorder {
	t.Helper()
	_, revision := f.service.Current()
	payload, err := json.Marshal(contract.IdentityTrial{
		Revision: revision,
		Identity: contract.IdentityInput{
			Issuer:       candidate.URL,
			ClientId:     "console",
			Ca:           testProviderCA(candidate),
			GroupsClaim:  "groups",
			ClientSecret: new("candidate-secret"),
		},
		AccessRules: rules,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/settings/identity/trial", bytes.NewReader(payload))
	request.AddCookie(f.session)
	request.Header.Set("Origin", "https://console.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	return response
}

// complete signs in through candidate to finish a started trial.
func (f *trialFixture) complete(t *testing.T, candidate *httptest.Server, trial *httptest.ResponseRecorder) {
	t.Helper()
	if trial.Code != http.StatusOK {
		t.Fatalf("trial failed: %d %s", trial.Code, trial.Body.String())
	}
	var redirect contract.LoginRedirect
	if err := json.Unmarshal(trial.Body.Bytes(), &redirect); err != nil {
		t.Fatal(err)
	}
	completeTestLogin(t, candidate, f.authentication, redirect.Url, trial.Result().Cookies())
}

func (f *trialFixture) lastChange(t *testing.T) string {
	t.Helper()
	changes, err := f.service.History(context.Background())
	if err != nil || len(changes) == 0 {
		t.Fatal("settings history is unavailable")
	}
	return changes[len(changes)-1].Summary
}

func TestIdentityTrialWithTheSameIssuerKeepsAccessWithoutADecision(t *testing.T) {
	f := newTrialFixture(t)
	if response := f.trial(t, f.active, new(contract.CarryOver)); response.Code != http.StatusBadRequest {
		t.Fatalf("same-issuer trial accepted an access rules decision: %d", response.Code)
	}
	_, revision := f.service.Current()
	trial := f.trial(t, f.active, nil)
	if before, _ := f.service.Current(); before.Identity.ClientSecret != "private-client-secret" {
		t.Fatal("identity activated before login")
	}
	f.complete(t, f.active, trial)
	after, afterRevision := f.service.Current()
	if after.Identity.ClientSecret != "candidate-secret" || after.AccessIssuer != f.active.URL || afterRevision != revision+1 ||
		len(after.Users) != 2 || len(after.Groups) != 1 {
		t.Fatal("same-issuer activation did not keep access rules")
	}
	if summary := f.lastChange(t); summary != "Identity provider verified and activated" {
		t.Fatalf("summary = %q", summary)
	}
	previous := httptest.NewRequest(http.MethodGet, "/", nil)
	previous.AddCookie(f.session)
	if _, allowed := f.authentication.Viewer(previous); allowed {
		t.Fatal("provider activation retained old sessions")
	}
}

func TestIdentityTrialWithANewIssuerRequiresAnAccessRulesDecision(t *testing.T) {
	f := newTrialFixture(t)
	// A provider that reuses the numeric subject of an existing binding.
	candidate := signedTestProviderFor(t, "42")
	defer candidate.Close()
	_, revision := f.service.Current()
	response := f.trial(t, candidate, nil)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "issuer differs") {
		t.Fatalf("issuer change without a decision = %d %s", response.Code, response.Body.String())
	}
	if invalid := f.trial(t, candidate, new(contract.IdentityTrialAccessRules("keep"))); invalid.Code != http.StatusBadRequest {
		t.Fatal("unknown access rules decision accepted")
	}
	if current, afterRevision := f.service.Current(); afterRevision != revision || current.AccessIssuer != f.active.URL {
		t.Fatal("rejected trial changed settings")
	}
}

func TestIdentityTrialWithANewIssuerCarriesOverAccess(t *testing.T) {
	f := newTrialFixture(t)
	candidate := signedTestProviderFor(t, "gitlab-admin")
	defer candidate.Close()
	trial := f.trial(t, candidate, new(contract.CarryOver))
	if before, _ := f.service.Current(); before.AccessIssuer != f.active.URL {
		t.Fatal("access moved to the new issuer before login")
	}
	f.complete(t, candidate, trial)
	after, _ := f.service.Current()
	if after.Identity.Issuer != candidate.URL || after.AccessIssuer != candidate.URL || len(after.Users) != 3 || len(after.Groups) != 1 {
		t.Fatal("carry over did not keep access rules under the new issuer")
	}
	if user, allowed := after.Resolve(config.Principal{Issuer: candidate.URL, Subject: "gitlab-admin"}); !allowed || !user.Admin {
		t.Fatal("trial administrator was not enrolled under the new issuer")
	}
	if user, allowed := after.Resolve(
		config.Principal{Issuer: candidate.URL, Subject: "42"},
	); !allowed || user.Admin ||
		len(user.Scopes) != 1 {
		t.Fatal("carried-over binding does not apply to the new issuer")
	}
	if _, allowed := after.Resolve(config.Principal{Issuer: f.active.URL, Subject: "engineer"}); allowed {
		t.Fatal("binding still applies to the previous issuer")
	}
	if summary := f.lastChange(t); !strings.Contains(summary, "carried over") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestIdentityTrialWithANewIssuerRemovesAccess(t *testing.T) {
	f := newTrialFixture(t)
	candidate := signedTestProviderFor(t, "gitlab-admin")
	defer candidate.Close()
	f.complete(t, candidate, f.trial(t, candidate, new(contract.Remove)))
	after, _ := f.service.Current()
	if after.AccessIssuer != candidate.URL || len(after.Groups) != 0 || len(after.Users) != 1 ||
		after.Users[0].Subject != "gitlab-admin" || !after.Users[0].Admin {
		t.Fatal("removal kept access rules or skipped the trial administrator")
	}
	for _, principal := range []config.Principal{
		{Issuer: candidate.URL, Subject: "42"},
		{Issuer: candidate.URL, Subject: "someone", Groups: []string{"platform"}},
	} {
		if _, allowed := after.Resolve(principal); allowed {
			t.Fatalf("removed binding matched %q", principal.Subject)
		}
	}
	if summary := f.lastChange(t); !strings.Contains(summary, "removed") {
		t.Fatalf("summary = %q", summary)
	}
}

func TestActivateIdentityVerifiesTheTrialPrincipal(t *testing.T) {
	const issuer = "https://identity.example"
	current := config.Config{Users: []config.User{{Subject: "admin", Admin: true}, {Subject: "reader"}}}
	current.SeedIdentity(config.Identity{Issuer: issuer})
	candidate := config.Identity{Issuer: issuer, ClientID: "rotated"}
	if _, _, err := activateIdentity(current, candidate, "", config.Principal{Issuer: issuer, Subject: "reader"}); err == nil {
		t.Fatal("same-issuer activation accepted a non-administrator login")
	}
	if _, _, err := activateIdentity(
		current,
		candidate,
		"",
		config.Principal{Issuer: "https://other.example", Subject: "admin"},
	); err == nil {
		t.Fatal("activation accepted a login from another issuer")
	}
	other := config.Identity{Issuer: "https://other.example"}
	if _, _, err := activateIdentity(current, other, "", config.Principal{Issuer: other.Issuer, Subject: "admin"}); err == nil {
		t.Fatal("issuer change activated without a decision")
	}
}

func testProviderCA(provider *httptest.Server) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: provider.Certificate().Raw}))
}

func testAuthenticator(t *testing.T, provider *httptest.Server, service *settings.Service) *auth.Authenticator {
	t.Helper()
	identityProvider, err := auth.NewProvider(
		oidc.ClientContext(context.Background(), provider.Client()),
		provider.URL,
		"console",
		"secret",
		"https://console.example/auth/callback",
	)
	if err != nil {
		t.Fatal(err)
	}
	return auth.New(identityProvider, service.Resolve)
}

func completeTestLogin(
	t *testing.T,
	provider *httptest.Server,
	a *auth.Authenticator,
	location string,
	cookies []*http.Cookie,
) *http.Cookie {
	t.Helper()
	client := provider.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(location)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	callbackURL, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, callbackURL.String(), nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	callback := httptest.NewRecorder()
	a.Callback(callback, request)
	if callback.Code != 303 {
		t.Fatalf("callback failed: %d %s", callback.Code, callback.Body.String())
	}
	for _, cookie := range callback.Result().Cookies() {
		if cookie.Name == "__Host-console-session" {
			return cookie
		}
	}
	t.Fatal("callback did not issue a session")
	return nil
}
