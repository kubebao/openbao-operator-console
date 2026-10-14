package store

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestConnectNamesTheCauseOfTheLastAttempt(t *testing.T) {
	trusted := newTestCA(t)
	untrusted := newTestCA(t)
	now := time.Now()
	tests := []struct {
		name string
		host string
		ca   testCA
		leaf func() tls.Certificate
		want string
	}{
		{
			name: "unknown authority",
			ca:   untrusted,
			leaf: func() tls.Certificate { return trusted.issue(t, []net.IP{loopback}, nil, now.Add(time.Hour)) },
			want: "server certificate is not signed by a trusted CA",
		},
		{
			name: "host name mismatch",
			ca:   trusted,
			leaf: func() tls.Certificate { return trusted.issue(t, nil, []string{"db.invalid"}, now.Add(time.Hour)) },
			want: "server certificate does not match the host name",
		},
		{
			name: "expired",
			ca:   trusted,
			leaf: func() tls.Certificate { return trusted.issue(t, []net.IP{loopback}, nil, now.Add(-time.Hour)) },
			want: "server certificate is expired or not yet valid",
		},
		{
			name: "connection refused",
			host: closedAddress(t),
			ca:   trusted,
			want: "connection refused",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			host := test.host
			if test.leaf != nil {
				host = serveTLS(t, test.leaf())
			}
			dsn := "postgres://console-user:not-logged@" + host + "/console?sslmode=verify-full&sslrootcert=" +
				test.ca.file(t)
			pool, err := pgxpool.New(context.Background(), dsn)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			err = connect(ctx, pool)
			if err == nil || !strings.HasPrefix(err.Error(), test.want+": ") {
				t.Fatalf("connect error = %v, want cause %q", err, test.want)
			}
			if strings.Contains(err.Error(), "console-user") || strings.Contains(err.Error(), "not-logged") {
				t.Fatalf("connect error exposes connection parameters: %v", err)
			}
		})
	}
}

var loopback = net.IPv4(127, 0, 0, 1)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T) testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "fixture database CA"},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{cert: cert, key: key}
}

func (ca testCA) file(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ca.crt")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func (ca testCA) issue(t *testing.T, ips []net.IP, names []string, notAfter time.Time) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "fixture database"},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     notAfter,
		IPAddresses:  ips,
		DNSNames:     names,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// serveTLS accepts PostgreSQL SSLRequest messages and completes the TLS handshake
// with cert. It never speaks the PostgreSQL protocol after the handshake.
func serveTLS(t *testing.T, cert tls.Certificate) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	config := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				request := make([]byte, 8)
				if _, err := io.ReadFull(conn, request); err != nil {
					return
				}
				if _, err := conn.Write([]byte("S")); err != nil {
					return
				}
				_ = tls.Server(conn, config).Handshake()
			}()
		}
	}()
	return listener.Addr().String()
}

func closedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}
