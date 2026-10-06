package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/auth"
	"github.com/dc-tec/openbao-operator-console/internal/config"
	"github.com/dc-tec/openbao-operator-console/internal/contract"
	"github.com/dc-tec/openbao-operator-console/internal/settings"
)

const setupTestToken = "a1042a15bc3a01f189e840797934e107cb7228f2ab29f8943c67d6e73042bf4a"

type setupFixture struct {
	setup          *Setup
	service        *settings.Service
	repository     *configurationRepository
	authentication *auth.Authenticator
	handler        http.Handler
}

func pendingSetup(t *testing.T) setupFixture {
	t.Helper()
	repository := &configurationRepository{}
	service, err := settings.Open(context.Background(), repository, bytes.Repeat([]byte{3}, 32), nil)
	if err != nil {
		t.Fatal(err)
	}
	authentication := auth.NewPending()
	authentication.SetResolver(service.Resolve)
	setup, err := NewSetup(service, authentication, []byte(setupTestToken), "https://console.example/auth/callback", config.Config{MaxBackupAgeHours: 24, StartupGraceMinutes: 10})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Repository: &memoryRepository{}, Auth: authentication, Settings: service, Setup: setup, Origin: "https://console.example"}
	return setupFixture{setup: setup, service: service, repository: repository, authentication: authentication, handler: server.Handler()}
}

func setupRequest(handler http.Handler, method, path string, body []byte, origin string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "https://console.example"+path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", origin)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func unlockedSetup(t *testing.T, fixture setupFixture) *http.Cookie {
	t.Helper()
	payload, _ := json.Marshal(contract.SetupUnlock{Token: setupTestToken})
	response := setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, "https://console.example")
	if response.Code != http.StatusOK {
		t.Fatalf("unlock = %d: %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == setupCookie {
			if !cookie.Secure || !cookie.HttpOnly || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge <= 0 || cookie.MaxAge > 900 {
				t.Fatal("setup cookie is not protected or bounded")
			}
			return cookie
		}
	}
	t.Fatal("setup cookie was not created")
	return nil
}

func beginSetupTrial(t *testing.T, fixture setupFixture, provider *httptest.Server, cookie *http.Cookie) (*httptest.ResponseRecorder, contract.LoginRedirect) {
	t.Helper()
	payload, _ := json.Marshal(contract.IdentityInput{Issuer: provider.URL, ClientId: "console", ClientSecret: new("setup-provider-secret"), Ca: testProviderCA(provider), GroupsClaim: "groups"})
	response := setupRequest(fixture.handler, "POST", "/api/v1/setup/identity", payload, "https://console.example", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("begin trial = %d: %s", response.Code, response.Body.String())
	}
	var redirect contract.LoginRedirect
	if err := json.Unmarshal(response.Body.Bytes(), &redirect); err != nil {
		t.Fatal(err)
	}
	return response, redirect
}

func setupCallback(t *testing.T, fixture setupFixture, provider *httptest.Server, trial *httptest.ResponseRecorder, redirect contract.LoginRedirect) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	client := provider.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Get(redirect.Url)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	request := httptest.NewRequest("GET", response.Header.Get("Location"), nil)
	for _, cookie := range trial.Result().Cookies() {
		request.AddCookie(cookie)
	}
	callback := httptest.NewRecorder()
	fixture.handler.ServeHTTP(callback, request)
	return callback, request
}

func TestSetupUnlockOriginExpiryAndRateLimits(t *testing.T) {
	fixture := pendingSetup(t)
	payload, _ := json.Marshal(contract.SetupUnlock{Token: setupTestToken})
	for _, origin := range []string{"", "https://untrusted.example"} {
		if response := setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, origin); response.Code != 403 {
			t.Fatal("setup accepted an absent or foreign origin")
		}
	}
	for _, path := range []string{"/api/v1/snapshot", "/api/v1/settings", "/api/v1/deployments/private/history"} {
		if response := setupRequest(fixture.handler, "GET", path, nil, ""); response.Code != 503 || !strings.Contains(response.Body.String(), "setup_required") {
			t.Fatal("fleet API was served before initialization")
		}
	}
	cookie := unlockedSetup(t, fixture)
	response := setupRequest(fixture.handler, "GET", "/api/v1/setup", nil, "", cookie)
	var status contract.SetupStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.State != "unlocked" || status.SessionExpiresAt == nil || status.CallbackUrl != "https://console.example/auth/callback" {
		t.Fatal("unlocked status omitted safe setup metadata")
	}
	if strings.Contains(response.Body.String(), setupTestToken) || fixture.service.Initialized() {
		t.Fatal("unlock exposed token or initialized configuration")
	}
	now := fixture.setup.now().Add(16 * time.Minute)
	fixture.setup.now = func() time.Time { return now }
	response = setupRequest(fixture.handler, "GET", "/api/v1/setup", nil, "", cookie)
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.State != "required" || status.SessionExpiresAt != nil {
		t.Fatal("expired browser ticket retained setup access")
	}
	for range 5 {
		response = setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", []byte(`{"token":"wrong"}`), "https://console.example")
		if response.Code != 401 {
			t.Fatal("wrong token was accepted or valid attempts were limited too early")
		}
	}
	response = setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, "https://console.example")
	if response.Code != 429 || response.Header().Get("Retry-After") != "60" {
		t.Fatal("setup attempts were not rate limited")
	}
	now = now.Add(time.Minute)
	if response = setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, "https://console.example"); response.Code != 200 {
		t.Fatal("rate limit did not permit a later retry")
	}
	now = now.Add(time.Hour)
	response = setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, "https://console.example")
	if response.Code != 410 {
		t.Fatal("expired setup token remained usable")
	}
}

func TestSetupClaimRequiresVerifiedLoginAndClosesPermanently(t *testing.T) {
	provider := signedTestProvider(t)
	defer provider.Close()
	fixture := pendingSetup(t)
	cookie := unlockedSetup(t, fixture)
	trial, redirect := beginSetupTrial(t, fixture, provider, cookie)
	if fixture.service.Initialized() || fixture.repository.record.Revision != 0 {
		t.Fatal("candidate configuration was saved before verified login")
	}
	callback, request := setupCallback(t, fixture, provider, trial, redirect)
	if callback.Code != 303 || callback.Header().Get("Location") != "/settings/clusters/new" {
		t.Fatalf("verified setup callback = %d: %s", callback.Code, callback.Body.String())
	}
	current, revision := fixture.service.Current()
	user, allowed := fixture.service.Resolve(context.Background(), "engineer", nil)
	if revision != 1 || !allowed || !user.Admin || current.Identity == nil || current.Identity.Issuer != provider.URL || len(fixture.repository.changes) != 1 {
		t.Fatal("verified subject did not become the first administrator")
	}
	if bytes.Contains(fixture.repository.record.Payload, []byte("setup-provider-secret")) || bytes.Contains(fixture.repository.record.Payload, []byte(setupTestToken)) {
		t.Fatal("setup or provider credential persisted as plaintext")
	}
	var session *http.Cookie
	for _, candidate := range callback.Result().Cookies() {
		if candidate.Name == "__Host-console-session" {
			session = candidate
		}
	}
	if session == nil {
		t.Fatal("verified setup did not issue a normal console session")
	}
	if response := setupRequest(fixture.handler, "GET", "/api/v1/settings", nil, "", session); response.Code != 200 {
		t.Fatal("first administrator could not immediately use settings")
	}
	payload, _ := json.Marshal(contract.SetupUnlock{Token: setupTestToken})
	if response := setupRequest(fixture.handler, "POST", "/api/v1/setup/unlock", payload, "https://console.example", cookie); response.Code != 409 {
		t.Fatal("claimed setup token was accepted again")
	}
	replay := httptest.NewRecorder()
	fixture.handler.ServeHTTP(replay, request)
	if replay.Code != 400 {
		t.Fatal("consumed verified callback was replayed")
	}
	if _, revision := fixture.service.Current(); revision != 1 {
		t.Fatal("replay changed stored configuration")
	}
	restarted, err := settings.Open(context.Background(), fixture.repository, bytes.Repeat([]byte{3}, 32), nil)
	if err != nil || !restarted.Initialized() {
		t.Fatal("restart reopened a claimed database")
	}
	if _, err := NewSetup(restarted, auth.NewPending(), []byte(setupTestToken), "https://console.example/auth/callback", config.Config{MaxBackupAgeHours: 24}); err == nil {
		t.Fatal("initialized database accepted a new setup controller")
	}
}

func TestSetupOutstandingLoginRejectsLockedExpiredAndSupersededSessions(t *testing.T) {
	for _, scenario := range []string{"locked", "expired", "superseded"} {
		t.Run(scenario, func(t *testing.T) {
			provider := signedTestProvider(t)
			defer provider.Close()
			fixture := pendingSetup(t)
			cookie := unlockedSetup(t, fixture)
			trial, redirect := beginSetupTrial(t, fixture, provider, cookie)
			switch scenario {
			case "locked":
				response := setupRequest(fixture.handler, "POST", "/api/v1/setup/lock", nil, "https://console.example", cookie)
				if response.Code != 200 || !strings.Contains(response.Body.String(), `"state":"required"`) {
					t.Fatal("lock did not close the browser ticket")
				}
			case "expired":
				future := fixture.setup.now().Add(16 * time.Minute)
				fixture.setup.now = func() time.Time { return future }
			case "superseded":
				beginSetupTrial(t, fixture, provider, cookie)
			}
			callback, _ := setupCallback(t, fixture, provider, trial, redirect)
			if callback.Code != 303 || callback.Header().Get("Location") != "/setup?result=failed" || fixture.service.Initialized() {
				t.Fatal("unavailable setup claim activated the candidate")
			}
		})
	}
}

func TestSetupCandidateErrorsAreSafeAndTokenMustBeRandom(t *testing.T) {
	fixture := pendingSetup(t)
	for _, token := range []string{"password", strings.Repeat("0", 64), strings.Repeat("z", 64)} {
		if _, err := NewSetup(fixture.service, fixture.authentication, []byte(token), "https://console.example/auth/callback", config.Config{MaxBackupAgeHours: 24}); err == nil {
			t.Fatal("weak or invalid setup token accepted")
		}
	}
	cookie := unlockedSetup(t, fixture)
	payload, _ := json.Marshal(contract.IdentityInput{Issuer: "http://untrusted.example", ClientId: "console", ClientSecret: new("never-echo-this-secret"), GroupsClaim: "groups"})
	response := setupRequest(fixture.handler, "POST", "/api/v1/setup/identity", payload, "https://console.example", cookie)
	if response.Code != 400 || strings.Contains(response.Body.String(), "never-echo-this-secret") || fixture.service.Initialized() {
		t.Fatal("discovery error saved or exposed candidate identity")
	}
}
