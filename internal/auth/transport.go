package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// ProviderContext adds an optional private CA without disabling TLS verification.
// The transport is used for discovery, token exchange, and signature keys.
func ProviderContext(ctx context.Context, caFile string) (context.Context, error) {
	data := []byte{}
	if caFile != "" {
		var err error
		data, err = os.ReadFile(caFile)
		if err != nil {
			return nil, errors.New("read OIDC CA file")
		}
	}
	return providerContext(ctx, data)
}

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

// Discover builds an OIDC authenticator for an identity provider, trusting its optional
// CA in addition to the system roots.
func Discover(ctx context.Context, identity config.Identity, redirect string, users []config.User) (*Authenticator, error) {
	providerContext, err := providerContext(ctx, []byte(identity.CA))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrProviderCA, err)
	}
	authenticator, err := NewOIDC(providerContext, identity.Issuer, identity.ClientID, identity.ClientSecret, redirect, users)
	if err != nil {
		return nil, err
	}
	authenticator.SetGroupsClaim(identity.GroupsClaim)
	return authenticator, nil
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
