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

func TestOIDCLoginValidatesPKCENonceEnrollmentAndReplay(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}
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
			var issuer, nonce, challenge string
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
					Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}},
				})
			})
			mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
				if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
					t.Error("PKCE verifier did not match login challenge")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				claimNonce := nonce
				if scenario.wrongNonce {
					claimNonce = "different"
				}
				token, err := jwt.Signed(signer).
					Claims(map[string]any{
						"iss":   issuer,
						"aud":   "console",
						"sub":   scenario.subject,
						"nonce": claimNonce,
						"exp":   time.Now().Add(time.Hour).Unix(),
						"iat":   time.Now().Unix(),
					}).
					Serialize()
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
			})
			provider := httptest.NewTLSServer(mux)
			defer provider.Close()
			issuer = provider.URL
			ctx := oidc.ClientContext(context.Background(), provider.Client())
			a, err := NewOIDC(
				ctx,
				issuer,
				"console",
				"test-client-secret",
				"https://console.example/auth/callback",
				[]config.User{{Subject: "operator", Name: "Engineer"}},
			)
			if err != nil {
				t.Fatal(err)
			}
			login := httptest.NewRecorder()
			a.Login(login, httptest.NewRequest(http.MethodGet, "https://console.example/auth/login", nil))
			location, err := url.Parse(login.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			query := location.Query()
			nonce, challenge = query.Get("nonce"), query.Get("code_challenge")
			if nonce == "" || challenge == "" || query.Get("code_challenge_method") != "S256" {
				t.Fatal("login did not bind nonce and PKCE")
			}
			cookies := login.Result().Cookies()
			if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
				t.Fatal("state cookie protections missing")
			}
			target := "https://console.example/auth/callback?code=valid&state=" + query.Get("state")
			request := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
			request.AddCookie(cookies[0])
			callback := httptest.NewRecorder()
			a.Callback(callback, request)
			if callback.Code != http.StatusSeeOther || callback.Header().Get("Location") != scenario.location {
				t.Fatalf("callback = %d %q, want %q", callback.Code, callback.Header().Get("Location"), scenario.location)
			}
			if scenario.location == "/" {
				var sessionCookie *http.Cookie
				for _, cookie := range callback.Result().Cookies() {
					if cookie.Name == "__Host-console-session" {
						sessionCookie = cookie
					}
				}
				if sessionCookie == nil || !sessionCookie.Secure || !sessionCookie.HttpOnly {
					t.Fatal("session cookie missing protections")
				}
				view := httptest.NewRequest(http.MethodGet, "https://console.example/api/v1/snapshot", nil)
				view.AddCookie(sessionCookie)
				if user, ok := a.Viewer(view); !ok || user.Subject != "operator" {
					t.Fatal("session did not authenticate enrolled subject")
				}
				logout := httptest.NewRecorder()
				a.Logout(logout, view)
				if _, ok := a.Viewer(view); ok || logout.Header().Get("Location") != "/?signin=signed-out" {
					t.Fatal("logout retained session or skipped the signed-out notice")
				}
			}
			replay := httptest.NewRecorder()
			a.Callback(replay, request)
			if replay.Code != http.StatusSeeOther || replay.Header().Get("Location") != "/?signin=expired" {
				t.Fatal("consumed state allowed replay")
			}
		})
	}
}
