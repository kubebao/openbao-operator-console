package renewal

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestRealServiceAccountSelfRenewalAndNamedPermissionBoundary(t *testing.T) {
	path := os.Getenv("CONSOLE_TEST_RENEWAL_KUBECONFIG")
	if path == "" {
		t.Skip("requires isolated console-dev renewal reader")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read isolated renewal credential")
	}
	credential, err := config.ImportKubeconfig(data, "kind-console-dev")
	clear(data)
	if err != nil {
		t.Fatal("import isolated renewal credential")
	}
	connection := config.Connection{ID: "renewal-test", Name: "Renewal test", Remote: true, Credential: &credential, Renewal: &config.TokenRenewal{Namespace: "console-system", ServiceAccount: "console-renewal-test"}, Namespaces: []config.Namespace{{Name: "console-sample", Environment: "development"}}, Operators: []config.Operator{{ID: "operator", Namespace: "console-system", Deployment: "console-operator-openbao-operator-controller", Container: "manager", Namespaces: []string{"console-sample"}}}}
	if connection.ValidateRenewal() != nil {
		t.Fatal("refusing an unscoped or administrator credential")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	now := time.Now().UTC()
	issued, expires, err := (KubernetesIssuer{}).Issue(ctx, connection, now)
	if err != nil {
		t.Fatal("real scoped self-renewal failed")
	}
	if issued.Token == credential.Token || !expires.After(now.Add(2*time.Minute)) {
		t.Fatal("API did not replace the token with a usable lifetime")
	}
	client, err := rest.HTTPClientFor(&rest.Config{Host: issued.Server, BearerToken: issued.Token, Timeout: 10 * time.Second, TLSClientConfig: rest.TLSClientConfig{CAData: []byte(issued.CA)}})
	if err != nil {
		t.Fatal("configure renewed reader")
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	for _, boundary := range []struct{ method, path string }{
		{http.MethodPost, "/api/v1/namespaces/console-system/serviceaccounts/console-reader/token"},
		{http.MethodGet, "/api/v1/namespaces/console-system/secrets"},
		{http.MethodGet, "/apis/openbao.org/v1alpha1/namespaces/kube-system/openbaoclusters"},
		{http.MethodGet, "/apis/apps/v1/namespaces/console-system/deployments"},
	} {
		request, err := http.NewRequestWithContext(ctx, boundary.method, issued.Server+boundary.path, strings.NewReader(`{"apiVersion":"authentication.k8s.io/v1","kind":"TokenRequest","spec":{"expirationSeconds":600}}`))
		if err != nil {
			t.Fatal("construct boundary check")
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("boundary check unreachable")
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusForbidden {
			t.Fatalf("reader exceeded %s boundary: status %d", boundary.path, response.StatusCode)
		}
	}
	t.Log("Real TokenRequest self-renewal, scoped reads, and four denied access boundaries passed")
}
