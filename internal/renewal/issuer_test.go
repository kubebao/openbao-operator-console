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
	return "e30." + base64.RawURLEncoding.EncodeToString(
		[]byte(`{"sub":"system:serviceaccount:reader-system:reader","marker":"`+marker+`"}`),
	) + ".signature"
}

// tokenCluster is a fake Kubernetes API. It answers one TokenRequest with the configured
// status and serves empty OpenBao lists to the issued token.
type tokenCluster struct {
	status  int
	expires time.Time
	posts   int
	reads   int
}

func (c *tokenCluster) serve(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			c.issue(t, w, r)
			return
		}
		c.read(t, w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func (c *tokenCluster) issue(t *testing.T, w http.ResponseWriter, r *http.Request) {
	c.posts++
	if r.URL.Path != "/api/v1/namespaces/reader-system/serviceaccounts/reader/token" ||
		r.Header.Get("Authorization") != "Bearer "+serviceToken("original") {
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
	w.WriteHeader(c.status)
	if c.status != http.StatusCreated {
		_, _ = w.Write([]byte("private-kubernetes-error"))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": "authentication.k8s.io/v1",
		"kind":       "TokenRequest",
		"status":     map[string]any{"token": serviceToken("new"), "expirationTimestamp": c.expires},
	})
}

func (c *tokenCluster) read(t *testing.T, w http.ResponseWriter, r *http.Request) {
	c.reads++
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/apis/openbao.org/v1alpha1/namespaces/team/") ||
		r.Header.Get("Authorization") != "Bearer "+serviceToken("new") {
		t.Error("validation escaped read-only namespace scope")
	}
	kind := "OpenBaoClusterList"
	if strings.HasSuffix(r.URL.Path, "openbaorestores") {
		kind = "OpenBaoRestoreList"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": "openbao.org/v1alpha1",
		"kind":       kind,
		"items":      []any{},
		"metadata":   map[string]any{"resourceVersion": "100"},
	})
}

// renewableConnection enrolls one namespace and renews the reader-system/reader token.
func renewableConnection(server *httptest.Server) config.Connection {
	return config.Connection{
		ID:         "one",
		Name:       "One",
		Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
		Renewal:    &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"},
		Access: config.Access{Mode: config.AccessRemote, Credential: &config.Credential{
			Server: server.URL,
			Token:  serviceToken("original"),
			CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
		}},
	}
}

func TestIssuerValidatesAnIssuedTokenWithScopedReads(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	cluster := &tokenCluster{status: http.StatusCreated, expires: now.Add(time.Hour)}
	credential, expires, err := (KubernetesIssuer{}).Issue(context.Background(), renewableConnection(cluster.serve(t)), now)
	if err != nil || cluster.posts != 1 || cluster.reads != 3 {
		t.Fatalf("issue = %v after %d requests and %d reads", err, cluster.posts, cluster.reads)
	}
	if credential.Token != serviceToken("new") || !expires.Equal(now.Add(time.Hour)) {
		t.Fatal("issued token was not validated using the server's granted lifetime")
	}
}

func TestIssuerClassifiesRejectedTokenRequests(t *testing.T) {
	for _, test := range []struct {
		status int
		state  settings.RenewalState
	}{
		{http.StatusUnauthorized, settings.RenewalReconnectRequired},
		{http.StatusForbidden, settings.RenewalPermissionDenied},
		{http.StatusInternalServerError, settings.RenewalRetrying},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			now := time.Now().UTC()
			cluster := &tokenCluster{status: test.status}
			credential, _, err := (KubernetesIssuer{}).Issue(context.Background(), renewableConnection(cluster.serve(t)), now)
			var failure *Failure
			if !errors.As(err, &failure) || strings.Contains(err.Error(), "private-kubernetes-error") {
				t.Fatalf("issue = %v, want a failure without provider data", err)
			}
			if cluster.posts != 1 || cluster.reads != 0 || credential.Token != "" {
				t.Fatal("failed issuance retried or validated a missing token")
			}
			if failure.State != test.state {
				t.Fatalf("state = %s, want %s", failure.State, test.state)
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
	c := config.Connection{
		Renewal: &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"},
		Access: config.Access{Mode: config.AccessRemote, Credential: &config.Credential{
			Server: server.URL,
			Token:  serviceToken("original"),
			CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
		}},
	}
	if _, _, err := (KubernetesIssuer{}).Issue(context.Background(), c, time.Now()); err == nil || reached {
		t.Fatal("redirect forwarded renewal credentials")
	}
}

func TestIssuerRejectsInvalidCandidatesAndKeepsCurrentToken(t *testing.T) {
	for _, scenario := range []string{
		"wrong subject",
		"short lifetime",
		"same token",
		"malformed",
		"oversized",
		"candidate unauthorized",
		"candidate forbidden",
		"untrusted TLS",
	} {
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
					token = "e30." + base64.RawURLEncoding.EncodeToString(
						[]byte(`{"sub":"system:serviceaccount:other:reader"}`),
					) + ".signature"
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
				_ = json.NewEncoder(w).Encode(map[string]any{
					"apiVersion": "authentication.k8s.io/v1",
					"kind":       "TokenRequest",
					"status":     map[string]any{"token": token, "expirationTimestamp": expires},
				})
			}))
			defer server.Close()
			c := config.Connection{
				Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
				Renewal:    &config.TokenRenewal{Namespace: "reader-system", ServiceAccount: "reader"},
				Access: config.Access{Mode: config.AccessRemote, Credential: &config.Credential{
					Server: server.URL,
					Token:  serviceToken("original"),
					CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
				}},
			}
			if scenario == "untrusted TLS" {
				c.Access.Credential.CA = ""
			}
			credential, _, err := (KubernetesIssuer{}).Issue(context.Background(), c, now)
			var failure *Failure
			if !errors.As(err, &failure) || failure.State != settings.RenewalRetrying || credential.Token != "" ||
				c.Access.Credential.Token != serviceToken("original") {
				t.Fatal("invalid candidate activated or discarded the current credential")
			}
		})
	}
}
