package httpapi

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

var styleNonce = regexp.MustCompile(`style-src 'self' 'nonce-([A-Za-z0-9]+)';`)

func TestApplicationShellCarriesAFreshStyleNonce(t *testing.T) {
	static := fstest.MapFS{
		"index.html":    {Data: []byte(`<meta name="csp-nonce" content="__CSP_NONCE__" />`)},
		"assets/app.js": {Data: []byte(`console.log("app")`)},
	}
	handler := (&Server{}).secure(frontend(static))
	seen := map[string]bool{}
	for _, path := range []string{"/", "/index.html", "/fleet", "/deployments/payments"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: status %d", path, response.Code)
		}
		policy := response.Header().Get("Content-Security-Policy")
		match := styleNonce.FindStringSubmatch(policy)
		if match == nil || strings.Contains(policy, "unsafe-inline") {
			t.Fatalf("%s: policy %q", path, policy)
		}
		nonce := match[1]
		if seen[nonce] {
			t.Fatalf("%s: nonce %q was reused", path, nonce)
		}
		seen[nonce] = true
		if body := response.Body.String(); body != `<meta name="csp-nonce" content="`+nonce+`" />` {
			t.Fatalf("%s: body %q", path, body)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: Cache-Control %q", path, got)
		}
	}
}

func TestStaticAssetsAllowNoInlineStyles(t *testing.T) {
	static := fstest.MapFS{
		"index.html":    {Data: []byte(`__CSP_NONCE__`)},
		"assets/app.js": {Data: []byte(`console.log("app")`)},
	}
	handler := (&Server{}).secure(frontend(static))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if response.Code != http.StatusOK || response.Body.String() != `console.log("app")` {
		t.Fatalf("asset: status %d body %q", response.Code, response.Body.String())
	}
	if policy := response.Header().Get("Content-Security-Policy"); policy != contentSecurityPolicy("") ||
		strings.Contains(policy, "nonce") || strings.Contains(policy, "unsafe-inline") {
		t.Fatalf("asset policy %q", policy)
	}
	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset: status %d", missing.Code)
	}
}

func TestMissingShellIsUnavailable(t *testing.T) {
	handler := (&Server{}).secure(frontend(fstest.MapFS{}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/fleet", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", response.Code)
	}
	if strings.Contains(response.Header().Get("Content-Security-Policy"), "nonce") {
		t.Fatal("an error page received a style nonce")
	}
}
