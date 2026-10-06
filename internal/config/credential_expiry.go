package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type CredentialExpirySource string

const (
	ExpiryCertificate   CredentialExpirySource = "certificate"
	ExpiryDeclaredToken CredentialExpirySource = "declaredToken"
	ExpiryUnknown       CredentialExpirySource = "unknown"
)

type CredentialExpiry struct {
	ExpiresAt *time.Time
	Source    CredentialExpirySource
}

// Expiry reports certificate validity or an unverified token declaration. A JWT
// declaration does not prove that the credential is authentic or accepted.
func (c *Credential) Expiry() CredentialExpiry {
	unknown := CredentialExpiry{Source: ExpiryUnknown}
	if c == nil {
		return unknown
	}
	if c.Certificate != "" {
		pair, err := tls.X509KeyPair([]byte(c.Certificate), []byte(c.Key))
		if err != nil || len(pair.Certificate) == 0 {
			return unknown
		}
		certificate, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return unknown
		}
		expires := certificate.NotAfter.UTC()
		return CredentialExpiry{ExpiresAt: &expires, Source: ExpiryCertificate}
	}
	if len(c.Token) > 16384 {
		return unknown
	}
	parts := strings.Split(c.Token, ".")
	if len(parts) != 3 {
		return unknown
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return unknown
	}
	var declaration struct {
		Expiry int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &declaration) != nil || declaration.Expiry <= 0 || declaration.Expiry > 253402300799 {
		return unknown
	}
	expires := time.Unix(declaration.Expiry, 0).UTC()
	return CredentialExpiry{ExpiresAt: &expires, Source: ExpiryDeclaredToken}
}
