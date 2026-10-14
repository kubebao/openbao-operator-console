package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOriginComparesCanonicalOrigins(t *testing.T) {
	for _, test := range []struct {
		configured, origin string
		allowed            bool
	}{
		{"https://console.example", "https://console.example", true},
		{"https://Console.Example:443", "https://console.example", true},
		{"https://console.example", "HTTPS://CONSOLE.EXAMPLE:443", true},
		{"https://console.example:8443", "https://console.example:8443", true},
		{"https://[2001:db8::1]:443", "https://[2001:db8::1]", true},
		{"https://console.example:8443", "https://console.example", false},
		{"https://console.example", "http://console.example", false},
		{"https://console.example", "https://console.example.attacker.example", false},
		{"https://console.example", "null", false},
		{"https://console.example", "", false},
		{"https://console.example", "https://user@console.example", false},
		{"", "", false},
	} {
		request := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		request.Header.Set("Origin", test.origin)
		if got := (&Server{Origin: test.configured}).sameOrigin(request); got != test.allowed {
			t.Errorf("configured %q, origin %q: allowed = %v, want %v", test.configured, test.origin, got, test.allowed)
		}
	}
}

func TestStrictTransportSecurityFollowsThePublicOrigin(t *testing.T) {
	for _, test := range []struct {
		origin, header string
	}{
		{"https://console.example", "max-age=31536000"},
		{"HTTPS://console.example:443", "max-age=31536000"},
		{"http://127.0.0.1:8090", ""},
		{"", ""},
	} {
		handler := (&Server{Fleet: &memoryFleet{}, Auth: viewerAuth{}, Origin: test.origin}).Handler()
		for _, path := range []string{"/healthz", "/api/v1/snapshot", "/"} {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if got := response.Header().Get("Strict-Transport-Security"); got != test.header {
				t.Errorf("origin %q %s: HSTS = %q, want %q", test.origin, path, got, test.header)
			}
		}
	}
}
