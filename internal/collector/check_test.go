package collector

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestCheckDiagnosticClassifiesFailuresWithoutRawDetails(t *testing.T) {
	resource := schema.GroupResource{Group: "openbao.org", Resource: "openbaoclusters"}
	for _, test := range []struct {
		name  string
		err   error
		state DiagnosticState
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
	code := 403
	reader := testReader(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openbaorestores") {
			w.WriteHeader(code)
			return
		}
		writeList(w, "openbaoclusters", []any{}, "")
	})
	batch, diagnostic := reader.Check(context.Background(), time.Now())
	if diagnostic.State != "available" || batch.Connection.Collection != "current" || batch.Connection.Capabilities[1].State != "denied" {
		t.Fatal("optional denial blocked inventory check")
	}
	code = 401
	batch, diagnostic = reader.Check(context.Background(), time.Now())
	if diagnostic.State != "unauthorized" || batch.Connection.Collection != "denied" {
		t.Fatal("rejected credential became optional warning")
	}
}
