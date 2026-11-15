package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// FuzzSafeReturn checks the Location header that the browser receives, including
// the browser convention that treats backslashes as slashes in HTTP URLs.
func FuzzSafeReturn(f *testing.F) {
	for _, seed := range []string{
		"", "/", "/deployments/bao?tab=history", "//attacker.example", "/\\attacker.example",
		"///attacker.example", "/local\\attacker.example", "https://attacker.example",
		"/%2f%2fattacker.example", "/%5cattacker.example", "/\r/attacker.example", "/\n/attacker.example",
		"/auth/login", strings.Repeat("/", maxReturnTo+1),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		request := httptest.NewRequest(http.MethodGet, "https://console.example/deployments", nil)
		response := httptest.NewRecorder()
		http.Redirect(response, request, safeReturn(value), http.StatusSeeOther)
		location := response.Header().Get("Location")
		if len(location) == 0 {
			t.Fatal("redirect has no Location header")
		}
		browserPath := strings.ReplaceAll(location, "\\", "/")
		target, err := url.Parse(browserPath)
		if err != nil {
			t.Fatalf("redirect is not a valid URL: %v", err)
		}
		resolved := request.URL.ResolveReference(target)
		if resolved.Scheme != request.URL.Scheme || resolved.Host != request.URL.Host || resolved.User != nil {
			t.Fatalf("redirect leaves the console origin: %q", location)
		}
	})
}
