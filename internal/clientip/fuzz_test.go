package clientip

import "testing"

// FuzzForwardedFor checks that client-supplied entries cannot cross the first
// untrusted hop or override a direct peer outside the trusted proxy networks.
func FuzzForwardedFor(f *testing.F) {
	for _, seed := range []string{"", "192.0.2.66", "10.0.0.8, 192.0.2.66", "garbage", "::1", ",\r\n"} {
		f.Add(seed)
	}
	resolver, err := Parse([]string{"10.0.0.0/8"})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, supplied string) {
		forwarded := supplied + ", 198.51.100.7, 10.0.0.9"
		if got := resolver.Client(request("10.0.0.2:4000", forwarded)); got != "198.51.100.7" {
			t.Fatalf("client-supplied entries changed the first untrusted hop: %q", got)
		}
		if got := resolver.Client(request("203.0.113.5:4000", supplied)); got != "203.0.113.5" {
			t.Fatalf("forwarded header overrode the untrusted direct peer: %q", got)
		}
	})
}
