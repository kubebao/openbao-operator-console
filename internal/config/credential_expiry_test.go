package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestCredentialExpirySeparatesDeclarationsFromCertificates(t *testing.T) {
	for _, token := range []string{"opaque", "a.invalid.b", "a." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":"123"}`)) + ".b", "a." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":999999999999999}`)) + ".b"} {
		if result := (&Credential{Token: token}).Expiry(); result.Source != "unknown" || result.ExpiresAt != nil {
			t.Fatal("invalid or opaque token acquired an expiry")
		}
	}
	token := "a." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":1800000000,"sub":"must-not-escape"}`)) + ".not-a-signature"
	result := (&Credential{Token: token}).Expiry()
	if result.Source != "declaredToken" || result.ExpiresAt == nil || result.ExpiresAt.Unix() != 1800000000 {
		t.Fatal("token declaration was not distinguished from verified credential evidence")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	certificate, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: expires.Add(-48 * time.Hour), NotAfter: expires}, &x509.Certificate{SerialNumber: big.NewInt(1)}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	encodedKey, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	result = (&Credential{Certificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})), Key: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: encodedKey}))}).Expiry()
	if result.Source != "certificate" || result.ExpiresAt == nil || !result.ExpiresAt.Equal(expires) {
		t.Fatal("certificate validity was not projected")
	}
}
