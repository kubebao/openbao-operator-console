// Package kube builds Kubernetes API clients for embedded reader credentials.
package kube

import (
	"net/http"

	"k8s.io/client-go/rest"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

// RESTConfig returns the client configuration for an embedded reader credential.
func RESTConfig(credential config.Credential) *rest.Config {
	return &rest.Config{
		Host:        credential.Server,
		BearerToken: credential.Token,
		TLSClientConfig: rest.TLSClientConfig{
			CAData:   []byte(credential.CA),
			CertData: []byte(credential.Certificate),
			KeyData:  []byte(credential.Key),
		},
	}
}

// HTTPClient never follows a redirect, so a reader credential reaches only the configured
// API server.
func HTTPClient(restConfig *rest.Config) (*http.Client, error) {
	client, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, err
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, nil
}
