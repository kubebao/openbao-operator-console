package collector

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/kubebao/openbao-operator-console/internal/store"
)

// DiagnosticState classifies the required workload access check.
type DiagnosticState string

const (
	DiagnosticAvailable    DiagnosticState = "available"
	DiagnosticUnauthorized DiagnosticState = "unauthorized"
	DiagnosticForbidden    DiagnosticState = "forbidden"
	DiagnosticUnsupported  DiagnosticState = "unsupported"
	DiagnosticTLSError     DiagnosticState = "tlsError"
	DiagnosticUnreachable  DiagnosticState = "unreachable"
	DiagnosticUnknown      DiagnosticState = "unknown"
)

// CheckDiagnostic describes only the required workload access check. Optional
// API and named controller results remain on the returned connection.
type CheckDiagnostic struct {
	State   DiagnosticState
	Message string
}

// Check projects a preview without persisting observations. Missing or forbidden
// optional APIs remain capability warnings in both preview and collection.
func (r *Reader) Check(ctx context.Context, now time.Time) (store.Batch, CheckDiagnostic) {
	diagnostic := CheckDiagnostic{State: DiagnosticAvailable, Message: "OpenBaoCluster access verified in all enrolled namespaces."}
	batch := r.collect(ctx, now, &diagnostic)
	return batch, diagnostic
}

func checkDiagnostic(err error) CheckDiagnostic {
	switch {
	case apierrors.IsUnauthorized(err):
		return CheckDiagnostic{
			State:   DiagnosticUnauthorized,
			Message: "The Kubernetes API rejected the reader credentials. Replace the credential and check again.",
		}
	case apierrors.IsForbidden(err):
		return CheckDiagnostic{
			State: DiagnosticForbidden,
			Message: "The reader cannot list OpenBaoCluster resources in an enrolled namespace. " +
				"Apply the scoped reader RBAC and check again.",
		}
	case apierrors.IsNotFound(err):
		return CheckDiagnostic{
			State:   DiagnosticUnsupported,
			Message: "The OpenBaoCluster API is unavailable. Check that the operator CRDs are installed in the selected cluster.",
		}
	}
	var verification *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var invalidCertificate x509.CertificateInvalidError
	var record tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &verification) || errors.As(err, &unknownAuthority) || errors.As(err, &hostname) ||
		errors.As(err, &invalidCertificate) ||
		errors.As(err, &record) ||
		errors.As(err, &alert) {
		return CheckDiagnostic{
			State:   DiagnosticTLSError,
			Message: "TLS verification failed. Check the API server address, CA certificate, and certificate validity.",
		}
	}
	var network *net.OpError
	if errors.As(err, &network) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return CheckDiagnostic{
			State: DiagnosticUnreachable,
			Message: "The Kubernetes API could not be reached within the check deadline. " +
				"Check the address and network access from the console.",
		}
	}
	return CheckDiagnostic{
		State:   DiagnosticUnknown,
		Message: "The Kubernetes API check failed. Check the reader configuration and API availability, then retry.",
	}
}
