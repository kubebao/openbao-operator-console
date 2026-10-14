package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// oidcProvider is a fake identity provider that checks PKCE and signs ID tokens for one
// subject. A wrong nonce simulates a token minted for another login.
type oidcProvider struct {
	server           *httptest.Server
	signer           jose.Signer
	subject          string
	wrongNonce       bool
	nonce, challenge string
}

func newOIDCProvider(t *testing.T, subject string, wrongNonce bool) *oidcProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}
	provider := &oidcProvider{signer: signer, subject: subject, wrongNonce: wrongNonce}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		issuer := provider.server.URL
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
			Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}},
		})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) { provider.token(t, w, r) })
	provider.server = httptest.NewTLSServer(mux)
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *oidcProvider) token(t *testing.T, w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		t.Error(err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != p.challenge {
		t.Error("PKCE verifier did not match login challenge")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	nonce := p.nonce
	if p.wrongNonce {
		nonce = "different"
	}
	claims := map[string]any{
		"iss":   p.server.URL,
		"aud":   "console",
		"sub":   p.subject,
		"nonce": nonce,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	}
	token, err := jwt.Signed(p.signer).Claims(claims).Serialize()
	if err != nil {
		t.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token": "provider-token-never-in-browser",
		"token_type":   "Bearer",
		"id_token":     token,
	})
}

// beginLogin starts a login and returns the callback request the browser would send.
func (p *oidcProvider) beginLogin(t *testing.T, ctx context.Context, a *Authenticator) *http.Request {
	t.Helper()
	login := httptest.NewRecorder()
	a.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
	location, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	p.nonce, p.challenge = query.Get("nonce"), query.Get("code_challenge")
	if p.nonce == "" || p.challenge == "" || query.Get("code_challenge_method") != "S256" {
		t.Fatal("login did not bind nonce and PKCE")
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("state cookie protections missing")
	}
	target := "https://console.example/auth/callback?code=valid&state=" + query.Get("state")
	request := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	request.AddCookie(cookies[0])
	return request
}

func issuedSession(response *httptest.ResponseRecorder) *http.Cookie {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "__Host-console-session" {
			return cookie
		}
	}
	return nil
}

// assertSessionUntilLogout checks that the session authenticates the subject and that
// logout ends it.
func assertSessionUntilLogout(t *testing.T, a *Authenticator, callback *httptest.ResponseRecorder) {
	t.Helper()
	cookie := issuedSession(callback)
	if cookie == nil || !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("session cookie missing protections")
	}
	view := httptest.NewRequest(http.MethodGet, "https://console.example/api/v1/snapshot", nil)
	view.AddCookie(cookie)
	if user, ok := a.Viewer(view); !ok || user.Subject != "operator" {
		t.Fatal("session did not authenticate enrolled subject")
	}
	logout := httptest.NewRecorder()
	a.Logout(logout, view)
	if _, ok := a.Viewer(view); ok || logout.Header().Get("Location") != "/?signin=signed-out" {
		t.Fatal("logout retained session or skipped the signed-out notice")
	}
}

func TestOIDCLoginValidatesPKCENonceEnrollmentAndReplay(t *testing.T) {
	for _, scenario := range []struct {
		name, subject string
		wrongNonce    bool
		location      string
	}{
		{"enrolled", "operator", false, "/"},
		{"unenrolled", "other", false, "/?signin=denied"},
		{"nonce mismatch", "operator", true, "/?signin=failed"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			provider := newOIDCProvider(t, scenario.subject, scenario.wrongNonce)
			ctx := oidc.ClientContext(context.Background(), provider.server.Client())
			identityProvider, err := NewProvider(
				ctx,
				provider.server.URL,
				"console",
				"test-client-secret",
				"https://console.example/auth/callback",
			)
			if err != nil {
				t.Fatal(err)
			}
			enrolled := config.User{Subject: "operator", Name: "Engineer"}
			// The session carries the verified token issuer, so bindings can be issuer-scoped.
			a := New(identityProvider, func(_ context.Context, principal config.Principal) (config.User, bool) {
				return enrolled, principal.Issuer == provider.server.URL && principal.Subject == enrolled.Subject
			})
			request := provider.beginLogin(t, ctx, a)
			callback := httptest.NewRecorder()
			a.Callback(callback, request)
			if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != scenario.location {
				t.Fatalf("callback = %d %q, want %q", callback.Code, callback.Header().Get("Location"), scenario.location)
			}
			if scenario.location == "/" {
				assertSessionUntilLogout(t, a, callback)
			}
			replay := httptest.NewRecorder()
			a.Callback(replay, request)
			if replay.Code != http.StatusSeeOther || replay.Header().Get("Location") != "/?signin=expired" {
				t.Fatal("consumed state allowed replay")
			}
		})
	}
}

func newTestLogin(t *testing.T) (*oidcProvider, context.Context, *Authenticator) {
	t.Helper()
	provider := newOIDCProvider(t, "operator", false)
	ctx := oidc.ClientContext(context.Background(), provider.server.Client())
	identityProvider, err := NewProvider(ctx, provider.server.URL, "console", "secret", "https://console.example/auth/callback")
	if err != nil {
		t.Fatal(err)
	}
	a := New(identityProvider, func(_ context.Context, principal config.Principal) (config.User, bool) {
		return config.User{Subject: principal.Subject}, principal.Subject == "operator"
	})
	return provider, ctx, a
}

func TestLoginFloodDoesNotBlockSignIn(t *testing.T) {
	provider, ctx, a := newTestLogin(t)
	for range 5 * maxConsumed / 4 {
		flood := httptest.NewRecorder()
		a.Login(flood, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
		if flood.Code != http.StatusFound {
			t.Fatal("unauthenticated login request was refused")
		}
	}
	if len(a.logins.consumed) != 0 || len(a.logins.trials) != 0 {
		t.Fatal("unauthenticated login requests retained server state")
	}
	callback := httptest.NewRecorder()
	a.Callback(callback, provider.beginLogin(t, ctx, a))
	if callback.Header().Get("Location") != "/" || issuedSession(callback) == nil {
		t.Fatal("login flood blocked a real sign-in")
	}
}

func TestTamperedForeignExpiredAndReplayedLoginsFail(t *testing.T) {
	provider, ctx, a := newTestLogin(t)
	var events bytes.Buffer
	a.Logger = slog.New(slog.NewJSONHandler(&events, nil))
	request := provider.beginLogin(t, ctx, a)
	original, err := request.Cookie(stateCookie)
	if err != nil {
		t.Fatal(err)
	}
	callback := func(value string) string {
		t.Helper()
		attempt := httptest.NewRequest(http.MethodGet, request.URL.String(), nil).WithContext(ctx)
		attempt.AddCookie(&http.Cookie{Name: stateCookie, Value: value})
		response := httptest.NewRecorder()
		a.Callback(response, attempt)
		return response.Header().Get("Location")
	}
	tampered := []byte(original.Value)
	tampered[len(tampered)/2] ^= 1
	if callback(string(tampered)) != "/?signin=failed" {
		t.Fatal("tampered login cookie was accepted")
	}
	foreign, err := newSealer().seal(pendingLogin{State: request.URL.Query().Get("state"), Verifier: "v", Nonce: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if callback(foreign) != "/?signin=failed" {
		t.Fatal("login cookie sealed by another process was accepted")
	}
	a.now = func() time.Time { return time.Now().Add(loginLifetime + time.Second) }
	if callback(original.Value) != "/?signin=expired" {
		t.Fatal("expired login cookie was accepted")
	}
	a.now = time.Now
	if callback(original.Value) != "/" {
		t.Fatal("rejected attempts consumed a valid login")
	}
	if callback(original.Value) != "/?signin=expired" {
		t.Fatal("used login state was replayed")
	}
	logged := events.String()
	for _, reason := range []string{"state_invalid", "expired", "replayed_or_superseded"} {
		if !strings.Contains(logged, `"reason":"`+reason+`"`) {
			t.Fatalf("security log omitted %s", reason)
		}
	}
	if strings.Contains(logged, original.Value) || strings.Contains(logged, request.URL.Query().Get("state")) ||
		strings.Contains(logged, "provider-token-never-in-browser") {
		t.Fatal("security log contained login secrets")
	}
}

func TestReplaySetEvictsOldestInsteadOfRefusing(t *testing.T) {
	logins := newStore()
	now := time.Now()
	expires := now.Add(loginLifetime).Unix()
	for i := range maxConsumed + 10 {
		login := pendingLogin{State: base64.RawURLEncoding.EncodeToString([]byte{byte(i), byte(i >> 8), byte(i >> 16)}), Expires: expires}
		if _, ok := logins.consume(login, now); !ok {
			t.Fatal("a full replay set refused a new login")
		}
	}
	if len(logins.consumed) != maxConsumed || len(logins.order) != maxConsumed {
		t.Fatal("replay set exceeded its bound")
	}
	first, err := logins.beginTrial(trial{})
	if err != nil {
		t.Fatal(err)
	}
	for range maxTrials {
		if _, err := logins.beginTrial(trial{}); err != nil {
			t.Fatal("a full trial set refused a new trial")
		}
	}
	if len(logins.trials) != maxTrials {
		t.Fatal("trial set exceeded its bound")
	}
	if _, ok := logins.consume(pendingLogin{State: "trial", Trial: first, Expires: expires}, now); ok {
		t.Fatal("evicted trial remained usable")
	}
}
