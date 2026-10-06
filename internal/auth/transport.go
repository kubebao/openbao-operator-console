package auth

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// ProviderContext adds an optional private CA without disabling TLS verification.
// The transport is used for discovery, token exchange, and signature keys.
func ProviderContext(ctx context.Context, caFile string) (context.Context, error) {
	data := []byte{}
	if caFile != "" {
		var err error
		data, err = os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read OIDC CA file")
		}
	}
	return ProviderContextPEM(ctx, data)
}

func ProviderContextPEM(ctx context.Context, data []byte) (context.Context, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if len(data) > 0 {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("load system certificate roots")
		}
		if !roots.AppendCertsFromPEM(data) {
			return nil, fmt.Errorf("OIDC CA file contains no certificates")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(request *http.Request, _ []*http.Request) error {
		if !httpsEndpoint(request.URL) {
			return fmt.Errorf("OIDC redirect must use HTTPS")
		}
		return nil
	}}
	return oidc.ClientContext(ctx, client), nil
}

func httpsEndpoint(endpoint *url.URL) bool {
	return endpoint != nil && endpoint.Scheme == "https" && endpoint.Host != "" && endpoint.User == nil
}
