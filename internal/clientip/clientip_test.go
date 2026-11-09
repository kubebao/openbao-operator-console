package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func request(remote string, forwarded ...string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	for _, value := range forwarded {
		r.Header.Add("X-Forwarded-For", value)
	}
	return r
}

func TestParseRefusesCatchAllAndInvalidProxies(t *testing.T) {
	for _, value := range []string{"0.0.0.0/0", "::/0", "not-an-address", "10.0.0.0/33"} {
		if _, err := Parse([]string{value}); err == nil {
			t.Fatalf("%q was accepted", value)
		}
	}
	resolver, err := Parse([]string{" 10.0.0.0/8 ", "", "192.0.2.10", "2001:db8::/32"})
	if err != nil || len(resolver.trusted) != 3 {
		t.Fatalf("valid proxies were refused: %v", err)
	}
}

func TestClientWithoutTrustedProxiesIgnoresForwardedHeaders(t *testing.T) {
	var resolver Resolver
	if got := resolver.Client(request("203.0.113.5:4000", "198.51.100.1")); got != "203.0.113.5" {
		t.Fatalf("got %s", got)
	}
}

func TestClientWalksForwardedForFromTheRight(t *testing.T) {
	resolver, err := Parse([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		remote    string
		forwarded []string
		want      string
	}{
		{"untrusted peer", "203.0.113.5:4000", []string{"198.51.100.1"}, "203.0.113.5"},
		{"one proxy", "10.0.0.2:4000", []string{"198.51.100.1"}, "198.51.100.1"},
		{"forged left entry", "10.0.0.2:4000", []string{"192.0.2.66, 198.51.100.1"}, "198.51.100.1"},
		{"two trusted proxies", "10.0.0.2:4000", []string{"198.51.100.1, 10.0.0.9"}, "198.51.100.1"},
		{"repeated headers", "10.0.0.2:4000", []string{"192.0.2.66", "198.51.100.1"}, "198.51.100.1"},
		{"address with port", "10.0.0.2:4000", []string{"198.51.100.1:5555"}, "198.51.100.1"},
		{"IPv6 client", "10.0.0.2:4000", []string{"2001:db8::7"}, "2001:db8::7"},
		{"malformed entry", "10.0.0.2:4000", []string{"198.51.100.1, garbage"}, "10.0.0.2"},
		{"no header", "10.0.0.2:4000", nil, "10.0.0.2"},
		{"only proxies", "10.0.0.2:4000", []string{"10.0.0.7"}, "10.0.0.7"},
		{"mapped IPv4 peer", "[::ffff:10.0.0.2]:4000", []string{"198.51.100.1"}, "198.51.100.1"},
	}
	for _, c := range cases {
		if got := resolver.Client(request(c.remote, c.forwarded...)); got != c.want {
			t.Fatalf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestMiddlewareStoresTheClientAddress(t *testing.T) {
	resolver, _ := Parse([]string{"10.0.0.0/8"})
	var seen string
	handler := resolver.Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { seen = From(r) }))
	handler.ServeHTTP(httptest.NewRecorder(), request("10.0.0.2:4000", "198.51.100.1"))
	if seen != "198.51.100.1" {
		t.Fatalf("got %s", seen)
	}
	if got := From(request("203.0.113.5:4000")); got != "203.0.113.5" {
		t.Fatalf("fallback got %s", got)
	}
}
