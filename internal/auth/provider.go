package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// Provider is a discovered OIDC identity provider with HTTPS endpoints. It is immutable,
// so a login keeps the provider it started with.
type Provider struct {
	oauth       oauth2.Config
	verifier    *oidc.IDTokenVerifier
	client      *http.Client
	groupsClaim string
}

// NewProvider discovers an identity provider through the HTTP client in ctx. It reads
// groups from the "groups" claim.
func NewProvider(ctx context.Context, issuer, clientID, clientSecret, redirect string) (*Provider, error) {
	if err := ValidCallback(redirect); err != nil {
		return nil, err
	}
	identity, err := url.Parse(issuer)
	if err != nil || !httpsEndpoint(identity) || identity.RawQuery != "" || identity.Fragment != "" || clientID == "" {
		return nil, errors.New("OIDC requires an HTTPS issuer and client ID")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("discover OIDC provider")
	}
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if err := provider.Claims(&discovery); err != nil {
		return nil, errors.New("decode OIDC discovery")
	}
	for _, endpoint := range []string{provider.Endpoint().AuthURL, provider.Endpoint().TokenURL, discovery.JWKS} {
		parsed, err := url.Parse(endpoint)
		if err != nil || !httpsEndpoint(parsed) {
			return nil, errors.New("OIDC endpoints must use HTTPS")
		}
	}
	client, ok := ctx.Value(oauth2.HTTPClient).(*http.Client)
	if !ok || client == nil {
		client = http.DefaultClient
	}
	return &Provider{
		oauth: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  redirect,
			Scopes:       []string{oidc.ScopeOpenID, "profile"},
		},
		verifier:    provider.Verifier(&oidc.Config{ClientID: clientID}),
		client:      client,
		groupsClaim: "groups",
	}, nil
}

// providerContext adds an optional private CA without disabling TLS verification.
// The transport is used for discovery, token exchange, and signature keys.
func providerContext(ctx context.Context, data []byte) (context.Context, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if len(data) > 0 {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, errors.New("load system certificate roots")
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, errors.New("OIDC CA file contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if !httpsEndpoint(request.URL) {
				return errors.New("OIDC redirect must use HTTPS")
			}
			return nil
		},
	}
	return oidc.ClientContext(ctx, client), nil
}

// ErrProviderCA reports an identity provider CA that contains no usable certificates.
var ErrProviderCA = errors.New("identity provider CA is invalid")

// Discover builds the configured identity provider, trusting its optional CA in addition
// to the system roots.
func Discover(ctx context.Context, identity config.Identity, redirect string) (*Provider, error) {
	providerContext, err := providerContext(ctx, []byte(identity.CA))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrProviderCA, err)
	}
	provider, err := NewProvider(providerContext, identity.Issuer, identity.ClientID, identity.ClientSecret, redirect)
	if err != nil {
		return nil, err
	}
	provider.groupsClaim = identity.GroupsClaim
	return provider, nil
}

// ValidCallback accepts only an HTTPS callback URL ending in /auth/callback.
func ValidCallback(raw string) error {
	callback, err := url.Parse(raw)
	if err != nil || callback.Scheme != "https" || callback.Host == "" || callback.Path != "/auth/callback" ||
		callback.RawQuery != "" || callback.Fragment != "" || callback.User != nil {
		return errors.New("OIDC callback must be an HTTPS URL ending in /auth/callback")
	}
	return nil
}

func httpsEndpoint(endpoint *url.URL) bool {
	return endpoint != nil && endpoint.Scheme == "https" && endpoint.Host != "" && endpoint.User == nil
}
