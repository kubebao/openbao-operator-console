// Package renewal manages remote ServiceAccount credentials independently of
// workload observation. TokenRequest is its only Kubernetes write operation.
package renewal

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"k8s.io/client-go/rest"

	"github.com/kubebao/openbao-operator-console/internal/collector"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/settings"
)

type Failure struct{ State settings.RenewalState }

func (e *Failure) Error() string { return "ServiceAccount credential renewal failed" }

// KubernetesIssuer projects only a validated token and the API's expiration.
// It never logs or persists raw Kubernetes replies or errors.
type KubernetesIssuer struct{}

func (KubernetesIssuer) Issue(ctx context.Context, connection config.Connection, now time.Time) (config.Credential, time.Time, error) {
	//nolint:unparam // Keeps each failure return on one line.
	fail := func(state settings.RenewalState) (config.Credential, time.Time, error) {
		return config.Credential{}, time.Time{}, &Failure{State: state}
	}
	if connection.Credential == nil || connection.Renewal == nil || connection.ValidateRenewal() != nil ||
		connection.Credential.Validate() != nil {
		return fail(settings.RenewalReconnectRequired)
	}
	credential := *connection.Credential
	client, err := rest.HTTPClientFor(
		&rest.Config{
			Host:            credential.Server,
			BearerToken:     credential.Token,
			Timeout:         10 * time.Second,
			TLSClientConfig: rest.TLSClientConfig{CAData: []byte(credential.CA)},
		},
	)
	if err != nil {
		return fail(settings.RenewalRetrying)
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint, err := url.JoinPath(
		credential.Server,
		"api/v1/namespaces",
		connection.Renewal.Namespace,
		"serviceaccounts",
		connection.Renewal.ServiceAccount,
		"token",
	)
	if err != nil {
		return fail(settings.RenewalRetrying)
	}
	requestBody := struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Spec       struct {
			ExpirationSeconds int64 `json:"expirationSeconds"`
		} `json:"spec"`
	}{APIVersion: "authentication.k8s.io/v1", Kind: "TokenRequest"}
	requestBody.Spec.ExpirationSeconds = 86400
	body, err := json.Marshal(requestBody)
	if err != nil {
		return fail(settings.RenewalRetrying)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fail(settings.RenewalRetrying)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "openbao-operator-console/credential-renewal")
	response, err := client.Do(request)
	if err != nil {
		return fail(settings.RenewalRetrying)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode == http.StatusUnauthorized {
		return fail(settings.RenewalReconnectRequired)
	}
	if response.StatusCode == http.StatusForbidden {
		return fail(settings.RenewalPermissionDenied)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return fail(settings.RenewalRetrying)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return fail(settings.RenewalRetrying)
	}
	defer clear(data)
	var issued struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Status     struct {
			Token      string    `json:"token"`
			Expiration time.Time `json:"expirationTimestamp"`
		} `json:"status"`
	}
	if json.Unmarshal(data, &issued) != nil || issued.APIVersion != "authentication.k8s.io/v1" || issued.Kind != "TokenRequest" ||
		!issued.Status.Expiration.After(now.Add(2*time.Minute)) ||
		issued.Status.Token == credential.Token {
		return fail(settings.RenewalRetrying)
	}
	credential.Token = issued.Status.Token
	connection.Credential = &credential
	if connection.ValidateRenewal() != nil || credential.Validate() != nil {
		return fail(settings.RenewalRetrying)
	}
	reader := collector.Unconfigured(connection, evidence.Policy{MaxBackupAge: 24 * time.Hour})
	batch, _ := reader.Check(ctx, now)
	if batch.Connection.Collection != "current" {
		// A rejected candidate does not establish that the current token is
		// invalid. Keep it active and retry issuance while it remains usable.
		return fail(settings.RenewalRetrying)
	}
	return credential, issued.Status.Expiration.UTC(), nil
}
