package renewal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

func serviceToken(marker string) string {
	return "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"`+marker+`"}`)) + ".signature"
}

func TestIssuerOnlyCreatesNamedTokenThenValidatesScopedReads(t *testing.T) {
	for _, responseCode := range []int{201, 401, 403, 500} {
		t.Run(http.StatusText(responseCode), func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			posts, gets := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					posts++
					if r.URL.Path != "/api/v1/namespaces/reader-system/serviceaccounts/reader/token" || r.Header.Get("Authorization") != "Bearer "+serviceToken("original") {
						t.Error("token request escaped its declared identity")
					}
					var body struct {
						Spec struct {
							ExpirationSeconds int64 `json:"expirationSeconds"`
						} `json:"spec"`
					}
					if json.NewDecoder(r.Body).Decode(&body) != nil || body.Spec.ExpirationSeconds != 86400 {
						t.Error("unexpected token lifetime request")
					}
					w.WriteHeader(responseCode)
					if responseCode != 201 {
						_, _ = w.Write([]byte("private-kubernetes-error"))
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenRequest", "status": map[string]interface{}{"token": serviceToken("new"), "expirationTimestamp": now.Add(time.Hour)}})
					return
				}
				gets++
				if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/apis/openbao.org/v1alpha1/namespaces/team/") || r.Header.Get("Authorization") != "Bearer "+serviceToken("new") {
					t.Error("validation escaped read-only namespace scope")
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "openbao.org/v1alpha1", "kind": "List", "items": []interface{}{}, "metadata": map[string]interface{}{}})
			}))
			defer server.Close()
			c := config.Connection{ID: "one", Name: "One", Remote: true, Namespaces: []config.Namespace{{Name: "team", Environment: "production"}}, Renewal: &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"}, Credential: &config.Credential{Server: server.URL, Token: serviceToken("original"), CA: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))}}
			credential, expires, err := (KubernetesIssuer{}).Issue(context.Background(), c, now)
			if posts != 1 {
				t.Fatal("unexpected issuance calls")
			}
			if responseCode == 201 {
				if err != nil || gets != 3 || credential.Token != serviceToken("new") || !expires.Equal(now.Add(time.Hour)) {
					t.Fatal("issued token was not validated using the server's granted lifetime")
				}
			} else {
				var failure *Failure
				if !errors.As(err, &failure) || strings.Contains(err.Error(), "private-kubernetes-error") || gets != 0 || credential.Token != "" {
					t.Fatal("failed issuance leaked provider data or validated a missing token")
				}
				if responseCode == 401 && failure.State != settings.RenewalReconnectRequired {
					t.Fatal("rejected current token was retried")
				}
				if responseCode == 403 && failure.State != settings.RenewalPermissionDenied {
					t.Fatal("permission failure was not actionable")
				}
			}
		})
	}
}

func TestIssuerNeverFollowsCredentialBearingRedirect(t *testing.T) {
	reached := false
	destination := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
	defer destination.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	c := config.Connection{Remote: true, Renewal: &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"}, Credential: &config.Credential{Server: server.URL, Token: serviceToken("original"), CA: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))}}
	if _, _, err := (KubernetesIssuer{}).Issue(context.Background(), c, time.Now()); err == nil || reached {
		t.Fatal("redirect forwarded renewal credentials")
	}
}

func TestIssuerRejectsInvalidCandidatesAndKeepsCurrentToken(t *testing.T) {
	for _, scenario := range []string{"wrong subject", "short lifetime", "same token", "malformed", "oversized", "candidate unauthorized", "candidate forbidden", "untrusted TLS"} {
		t.Run(scenario, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					if scenario == "candidate forbidden" {
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","reason":"Forbidden","code":403}`))
						return
					}
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","reason":"Unauthorized","code":401}`))
					return
				}
				token, expires := serviceToken("new"), now.Add(time.Hour)
				switch scenario {
				case "wrong subject":
					token = "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"system:serviceaccount:other:reader"}`)) + ".signature"
				case "short lifetime":
					expires = now.Add(time.Minute)
				case "same token":
					token = serviceToken("original")
				case "malformed":
					_, _ = w.Write([]byte("invalid-json"))
					return
				case "oversized":
					_, _ = w.Write([]byte(strings.Repeat("x", 65537)))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"apiVersion": "authentication.k8s.io/v1", "kind": "TokenRequest", "status": map[string]interface{}{"token": token, "expirationTimestamp": expires}})
			}))
			defer server.Close()
			c := config.Connection{Remote: true, Namespaces: []config.Namespace{{Name: "team", Environment: "production"}}, Renewal: &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"}, Credential: &config.Credential{Server: server.URL, Token: serviceToken("original"), CA: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}))}}
			if scenario == "untrusted TLS" {
				c.Credential.CA = ""
			}
			credential, _, err := (KubernetesIssuer{}).Issue(context.Background(), c, now)
			var failure *Failure
			if !errors.As(err, &failure) || failure.State != settings.RenewalRetrying || credential.Token != "" || c.Credential.Token != serviceToken("original") {
				t.Fatal("invalid candidate activated or discarded the current credential")
			}
		})
	}
}
