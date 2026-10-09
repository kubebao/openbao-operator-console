package collector

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
)

func TestCheckDiagnosticClassifiesFailuresWithoutRawDetails(t *testing.T) {
	resource := schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"}
	for _, test := range []struct {
		name  string
		err   error
		state contract.ConnectionDiagnosticState
	}{
		{"unauthorized", apierrors.NewUnauthorized("private-server-message"), "unauthorized"},
		{"forbidden", apierrors.NewForbidden(resource, "private-namespace", errors.New("private-server-message")), "forbidden"},
		{"missing API", apierrors.NewNotFound(resource, "private-resource"), "unsupported"},
		{"untrusted CA", fmt.Errorf("wrapped: %w", x509.UnknownAuthorityError{}), "tlsError"},
		{"hostname mismatch", x509.HostnameError{}, "tlsError"},
		{"expired certificate", x509.CertificateInvalidError{}, "tlsError"},
		{"network", &net.OpError{Op: "dial", Err: errors.New("private-address")}, "unreachable"},
		{"deadline", context.DeadlineExceeded, "unreachable"},
		{"unknown", errors.New("private-server-message"), "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := checkDiagnostic(test.err)
			if result.State != test.state || result.Message == "" {
				t.Fatalf("diagnostic = %#v", result)
			}
		})
	}
}

func TestCheckOptionalWarningsAndCredentialFailures(t *testing.T) {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaoclusters"}:   "OpenBaoClusterList",
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorestores"}:   "OpenBaoRestoreList",
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorecoveries"}: "OpenBaoRecoveryList",
	})
	client.PrependReactor("list", "openbaorestores", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(action.GetResource().GroupResource(), "team", nil)
	})
	reader := &Reader{
		client:     client,
		connection: config.Connection{ID: "one", Namespaces: []config.Namespace{{Name: "team", Environment: "production"}}},
	}
	batch, diagnostic := reader.Check(context.Background(), time.Now())
	if diagnostic.State != "available" || batch.Connection.Collection != "current" || len(batch.Deployments) != 0 ||
		batch.Connection.Capabilities[1].State != "denied" {
		t.Fatal("empty cluster or optional denied API blocked the required access check")
	}
	client.PrependReactor("list", "openbaorecoveries", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("private-message")
	})
	batch, diagnostic = reader.Check(context.Background(), time.Now())
	if diagnostic.State != "unauthorized" || batch.Connection.Collection != "denied" {
		t.Fatal("rejected credential became an optional API warning")
	}
}
