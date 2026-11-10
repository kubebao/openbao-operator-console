package httpapi

import (
	"context"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/localauth"
)

const localPassword = "maple river lantern 42"

type localCredentialRepository struct {
	mu      sync.Mutex
	records map[string]localauth.Record
}

func (r *localCredentialRepository) GetLocalCredential(_ context.Context, id string) (localauth.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[id]
	if !ok {
		return localauth.Record{}, localauth.ErrNotFound
	}
	return record, nil
}

func (r *localCredentialRepository) CreateLocalCredential(_ context.Context, record localauth.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.records[record.AccountID]; exists {
		return localauth.ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *localCredentialRepository) UpdateLocalCredential(_ context.Context, record localauth.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.records[record.AccountID]; !ok || current.Revision != record.Revision-1 {
		return localauth.ErrConflict
	}
	r.records[record.AccountID] = record
	return nil
}

func (r *localCredentialRepository) DeleteLocalCredentials(_ context.Context, keep []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id := range r.records {
		if !strings.Contains(strings.Join(keep, "\x00"), id) {
			delete(r.records, id)
		}
	}
	return nil
}

// clock is a TOTP clock that tests advance one step at a time.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) step() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(30 * time.Second)
}

func newLocalAuthenticator(t *testing.T) (*localauth.Authenticator, *clock) {
	t.Helper()
	store, err := localauth.NewStore(&localCredentialRepository{records: map[string]localauth.Record{}}, []byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	hasher, err := localauth.NewHasher()
	if err != nil {
		t.Fatal(err)
	}
	authenticator := localauth.NewAuthenticator(store, hasher, localauth.NewThrottle())
	timer := &clock{now: time.Unix(1_800_000_000, 0)}
	authenticator.Now = timer.Now
	return authenticator, timer
}

type localConsole struct {
	setupFixture
	local  *localauth.Authenticator
	clock  *clock
	secret []byte
	codes  []string
	server *Server
	// session is the administrator's session from setup.
	session *http.Cookie
}

func decodeJSON[T any](t *testing.T, response interface{ Bytes() []byte }) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

// sessionCookie returns the session cookie that response sets.
func sessionCookie(response *http.Response) *http.Cookie {
	const name = "__Host-console-session"
	for _, cookie := range response.Cookies() {
		if cookie.Name == name && cookie.MaxAge >= 0 && cookie.Value != "" {
			return cookie
		}
	}
	return nil
}

// localSetup completes browser setup with a local administrator named admin.
func localSetup(t *testing.T) localConsole {
	t.Helper()
	fixture := pendingSetup(t)
	local, timer := newLocalAuthenticator(t)
	fixture.setup.Local = local
	server := &Server{
		Fleet:    &memoryFleet{},
		Auth:     fixture.authentication,
		Settings: fixture.service,
		Setup:    fixture.setup,
		Origin:   "https://console.example",
		Local:    local,
	}
	fixture.handler = server.Handler()
	cookie := unlockedSetup(t, fixture)
	response := setupRequest(fixture.handler, "POST", "/api/v1/setup/local/authenticator", nil, "https://console.example", cookie)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticator = %d: %s", response.Code, response.Body.String())
	}
	enrollment := decodeJSON[contract.AuthenticatorEnrollment](t, response.Body)
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Key)
	if err != nil || !strings.HasPrefix(enrollment.Uri, "otpauth://totp/") || enrollment.QrSize < 21 || enrollment.QrPath == "" {
		t.Fatalf("unexpected enrollment %+v", enrollment)
	}
	create := func(input contract.LocalAdministratorInput) *httptest.ResponseRecorder {
		payload, _ := json.Marshal(input)
		return setupRequest(fixture.handler, "POST", "/api/v1/setup/local", payload, "https://console.example", cookie)
	}
	code := localauth.TOTPCode(secret, timer.Now())
	if response := create(
		contract.LocalAdministratorInput{Username: "admin", Password: "short", Code: code},
	); response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "at least 15 characters") {
		t.Fatalf("weak password = %d: %s", response.Code, response.Body.String())
	}
	if response := create(
		contract.LocalAdministratorInput{Username: "Admin", Password: localPassword, Code: code},
	); response.Code != http.StatusBadRequest {
		t.Fatalf("uppercase username = %d", response.Code)
	}
	if response := create(
		contract.LocalAdministratorInput{Username: "admin", Password: localPassword, Code: "000000"},
	); response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), "authenticator code") {
		t.Fatalf("wrong code = %d: %s", response.Code, response.Body.String())
	}
	created := create(contract.LocalAdministratorInput{Username: "admin", Name: "First administrator", Password: localPassword, Code: code})
	if created.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", created.Code, created.Body.String())
	}
	result := decodeJSON[contract.LocalEnrollmentResult](t, created.Body)
	if len(result.RecoveryCodes) != localauth.RecoveryCodeCount {
		t.Fatal("recovery codes were not returned")
	}
	session := sessionCookie(created.Result())
	if session == nil || !session.Secure || !session.HttpOnly {
		t.Fatal("setup did not sign the administrator in")
	}
	snapshot := setupRequest(fixture.handler, "GET", "/api/v1/snapshot", nil, "https://console.example", session)
	if snapshot.Code != http.StatusOK || !strings.Contains(snapshot.Body.String(), `"subject":"local:admin"`) {
		t.Fatalf("snapshot as local administrator = %d: %s", snapshot.Code, snapshot.Body.String())
	}
	return localConsole{
		setupFixture: fixture,
		local:        local,
		clock:        timer,
		secret:       secret,
		codes:        result.RecoveryCodes,
		server:       server,
		session:      session,
	}
}

func TestLocalSetupInitializesTheConsoleOnce(t *testing.T) {
	console := localSetup(t)
	configuration, _ := console.service.Current()
	account, found := configuration.FindLocalAccount("admin")
	if !found || !account.Admin || account.Name != "First administrator" || configuration.Identity != nil {
		t.Fatalf("configuration after setup: %+v", configuration.LocalAccounts)
	}
	if _, err := console.local.Store.Load(context.Background(), account.ID); err != nil {
		t.Fatal("credential was not stored")
	}
	again := setupRequest(console.handler, "POST", "/api/v1/setup/local/authenticator", nil, "https://console.example")
	if again.Code != http.StatusConflict {
		t.Fatalf("setup after completion = %d", again.Code)
	}
}

func (c localConsole) signIn(username, password, code, origin string) *httptest.ResponseRecorder {
	payload, _ := json.Marshal(contract.LocalSignInInput{Username: username, Password: password, Code: code})
	return setupRequest(c.handler, "POST", "/api/v1/sign-in/local", payload, origin)
}

func TestLocalSignInRequiresPasswordAndFreshSecondFactor(t *testing.T) {
	console := localSetup(t)
	options := decodeJSON[contract.SignInOptions](t, setupRequest(console.handler, "GET", "/api/v1/sign-in", nil, "").Body)
	if !options.LocalAccounts || options.SingleSignOn {
		t.Fatalf("sign-in options = %+v", options)
	}
	console.clock.step()
	code := localauth.TOTPCode(console.secret, console.clock.Now())
	if response := console.signIn("admin", localPassword, code, "https://attacker.example"); response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin sign-in = %d", response.Code)
	}
	rejected := []*httptest.ResponseRecorder{
		console.signIn("admin", "maple river lantern 41", code, "https://console.example"),
		console.signIn("admin", localPassword, "000000", "https://console.example"),
		console.signIn("nobody", localPassword, code, "https://console.example"),
	}
	for _, response := range rejected {
		if response.Code != http.StatusUnauthorized ||
			!strings.Contains(response.Body.String(), "username, password, or code is incorrect") {
			t.Fatalf("rejected sign-in = %d: %s", response.Code, response.Body.String())
		}
		if sessionCookie(response.Result()) != nil {
			t.Fatal("a rejected sign-in opened a session")
		}
	}
	response := console.signIn(" ADMIN ", localPassword, code, "https://console.example")
	if response.Code != http.StatusOK {
		t.Fatalf("sign-in = %d: %s", response.Code, response.Body.String())
	}
	session := sessionCookie(response.Result())
	if session == nil {
		t.Fatal("sign-in did not open a session")
	}
	if snapshot := setupRequest(console.handler, "GET", "/api/v1/snapshot", nil, "", session); snapshot.Code != http.StatusOK {
		t.Fatalf("snapshot = %d", snapshot.Code)
	}
	if replay := console.signIn("admin", localPassword, code, "https://console.example"); replay.Code != http.StatusUnauthorized {
		t.Fatalf("replayed code = %d", replay.Code)
	}
	recovery := console.signIn("admin", localPassword, console.codes[0], "https://console.example")
	result := decodeJSON[contract.LocalSignInResult](t, recovery.Body)
	if recovery.Code != http.StatusOK || !result.RecoveryCodeUsed || result.RecoveryCodesLeft != localauth.RecoveryCodeCount-1 {
		t.Fatalf("recovery sign-in = %d: %+v", recovery.Code, result)
	}
}

func TestLocalSignInDelaysRepeatedFailures(t *testing.T) {
	console := localSetup(t)
	for range 5 {
		console.signIn("admin", "wrong password value", "000000", "https://console.example")
	}
	response := console.signIn(
		"admin",
		localPassword,
		localauth.TOTPCode(console.secret, console.clock.Now().Add(30*time.Second)),
		"https://console.example",
	)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "30" {
		t.Fatalf("delayed sign-in = %d, Retry-After %q", response.Code, response.Header().Get("Retry-After"))
	}
	// Another username from the same client is not delayed by this account's failures.
	if other := console.signIn("someone", localPassword, "000000", "https://console.example"); other.Code != http.StatusUnauthorized {
		t.Fatalf("other account = %d", other.Code)
	}
}

func TestLocalSignInLimitsFailuresPerClient(t *testing.T) {
	console := localSetup(t)
	for i := range clientFailureLimit {
		console.signIn("user"+strings.Repeat("x", i), "wrong password value", "000000", "https://console.example")
	}
	if response := console.signIn(
		"fresh",
		"wrong password value",
		"000000",
		"https://console.example",
	); response.Code != http.StatusTooManyRequests {
		t.Fatalf("sign-in after client limit = %d", response.Code)
	}
}

func TestLocalSignInIsUnavailableWithoutLocalAccounts(t *testing.T) {
	console := localSetup(t)
	console.server.Local = nil
	handler := console.server.Handler()
	payload, _ := json.Marshal(contract.LocalSignInInput{Username: "admin", Password: localPassword, Code: "000000"})
	if response := setupRequest(
		handler,
		"POST",
		"/api/v1/sign-in/local",
		payload,
		"https://console.example",
	); response.Code != http.StatusNotFound {
		t.Fatalf("sign-in without local accounts = %d", response.Code)
	}
	options := decodeJSON[contract.SignInOptions](t, setupRequest(handler, "GET", "/api/v1/sign-in", nil, "").Body)
	if options.LocalAccounts {
		t.Fatal("sign-in options offered local accounts")
	}
}

func TestLocalAccountsResolveOnlyWhileEnrolled(t *testing.T) {
	console := localSetup(t)
	configuration, revision := console.service.Current()
	account, _ := configuration.FindLocalAccount("admin")
	configuration.LocalAccounts = append(configuration.LocalAccounts, config.LocalAccount{
		ID: "second", Username: "second", Admin: true, Scopes: []config.Scope{},
	})
	configuration.LocalAccounts = configuration.LocalAccounts[1:]
	if _, err := console.service.Save(context.Background(), revision, configuration, "test", "remove admin"); err != nil {
		t.Fatal(err)
	}
	if _, allowed := console.service.Resolve(
		context.Background(),
		config.Principal{Issuer: config.LocalIssuer, Subject: account.ID},
	); allowed {
		t.Fatal("a removed local account still resolved")
	}
	console.clock.step()
	if response := console.signIn(
		"admin",
		localPassword,
		localauth.TOTPCode(console.secret, console.clock.Now()),
		"https://console.example",
	); response.Code != http.StatusUnauthorized {
		t.Fatalf("removed account sign-in = %d", response.Code)
	}
}
