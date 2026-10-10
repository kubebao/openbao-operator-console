package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func sessionCookie(response *httptest.ResponseRecorder) *http.Cookie {
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
	cookie := sessionCookie(callback)
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
			a, err := NewOIDC(
				ctx,
				provider.server.URL,
				"console",
				"test-client-secret",
				"https://console.example/auth/callback",
				[]config.User{{Subject: "operator", Name: "Engineer"}},
			)
			if err != nil {
				t.Fatal(err)
			}
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
