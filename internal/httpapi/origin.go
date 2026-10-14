package httpapi

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// canonicalOrigin returns an HTTP(S) origin as scheme://host[:port] with the scheme and
// host in lower case and the scheme's default port removed. It returns an empty string
// for anything else, including the opaque "null" origin.
func canonicalOrigin(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return ""
	}
	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if (scheme != "https" && scheme != "http") || host == "" {
		return ""
	}
	port := parsed.Port()
	if (scheme == "https" && port == "443") || (scheme == "http" && port == "80") {
		port = ""
	}
	switch {
	case port != "":
		host = net.JoinHostPort(host, port)
	case strings.Contains(host, ":"):
		host = "[" + host + "]"
	}
	return scheme + "://" + host
}

// sameOrigin reports whether a state-changing request comes from the public origin.
// Both sides are canonicalized, so an explicit default port or a differently cased host
// in the configured callback URL does not reject every request.
func (s *Server) sameOrigin(r *http.Request) bool {
	expected := canonicalOrigin(s.Origin)
	return expected != "" && canonicalOrigin(r.Header.Get("Origin")) == expected
}
