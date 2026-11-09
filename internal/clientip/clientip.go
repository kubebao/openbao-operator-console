// Package clientip determines the client address of a request behind trusted reverse
// proxies. The address drives rate limits and security logs only; it never grants access.
package clientip

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const (
	// maxProxies bounds the trusted proxy list.
	maxProxies = 64
	// maxHops bounds how many X-Forwarded-For entries are examined.
	maxHops = 32
)

// Resolver trusts X-Forwarded-For only from proxies in its prefixes.
type Resolver struct {
	trusted []netip.Prefix
}

// Parse reads CIDR prefixes or single addresses. It refuses prefixes that match every
// address, because any client could then forge its address.
func Parse(values []string) (Resolver, error) {
	var result Resolver
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			addr, addrErr := netip.ParseAddr(value)
			if addrErr != nil {
				return Resolver{}, fmt.Errorf("trusted proxy %q is not an address or CIDR prefix", value)
			}
			prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
		}
		if prefix.Bits() == 0 {
			return Resolver{}, fmt.Errorf("trusted proxy %q matches every address; list the proxy networks", value)
		}
		result.trusted = append(result.trusted, prefix.Masked())
	}
	if len(result.trusted) > maxProxies {
		return Resolver{}, errors.New("too many trusted proxies")
	}
	return result, nil
}

func (r Resolver) trusts(addr netip.Addr) bool {
	for _, prefix := range r.trusted {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parseHop(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if addr, err := netip.ParseAddr(value); err == nil {
		return addr.Unmap(), true
	}
	if addrPort, err := netip.ParseAddrPort(value); err == nil {
		return addrPort.Addr().Unmap(), true
	}
	return netip.Addr{}, false
}

// Client returns the client address of request. Without a trusted direct peer, it is
// the peer itself. Behind trusted proxies, it walks X-Forwarded-For from the right and
// returns the first address that is not a trusted proxy. Entries left of that address
// come from the client and are ignored, so a client cannot forge its address.
func (r Resolver) Client(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil {
		host = request.RemoteAddr
	}
	client, ok := parseHop(host)
	if !ok {
		return host
	}
	if len(r.trusted) == 0 || !r.trusts(client) {
		return client.String()
	}
	hops := []string{}
	for _, header := range request.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(header, ",")...)
	}
	for i, examined := len(hops)-1, 0; i >= 0 && examined < maxHops && r.trusts(client); i, examined = i-1, examined+1 {
		hop, valid := parseHop(hops[i])
		if !valid {
			// A malformed entry ends the chain; the proxy that forwarded it is the
			// closest address known.
			break
		}
		client = hop
	}
	return client.String()
}

type contextKey struct{}

// Middleware stores the client address of each request for From.
func (r Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), contextKey{}, r.Client(request))
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

// From returns the client address that Middleware stored, or the direct peer address.
func From(request *http.Request) string {
	if value, ok := request.Context().Value(contextKey{}).(string); ok {
		return value
	}
	return Resolver{}.Client(request)
}
