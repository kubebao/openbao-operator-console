package config

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestRenewalRequiresMatchingServiceAccountAndScopedTokenGrant(t *testing.T) {
	connection := Connection{ID: "one", Name: "One", Remote: true, Namespaces: []Namespace{{Name: "team", Environment: "production"}}, Renewal: &TokenRenewal{Namespace: "console-system", ServiceAccount: "reader"}}
	var manifest bytes.Buffer
	if err := WriteRBAC(&manifest, connection, "reader", "console-system"); err != nil {
		t.Fatal(err)
	}
	text := manifest.String()
	if !strings.Contains(text, "- serviceaccounts/token") || !strings.Contains(text, "- create") || !strings.Contains(text, "resourceNames:\n  - reader") || strings.Contains(text, "secrets") || strings.Contains(text, "ClusterRole") {
		t.Fatal("renewal grant was not confined to one ServiceAccount")
	}
	manifest.Reset()
	if WriteRBAC(&manifest, connection, "other", "console-system") == nil || manifest.Len() != 0 {
		t.Fatal("RBAC generated for a different identity")
	}
	connection.Credential = &Credential{Server: "https://api.example", Token: "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"system:serviceaccount:console-system:reader"}`)) + ".signature"}
	if err := connection.ValidateRenewal(); err != nil {
		t.Fatal(err)
	}
	connection.Renewal.ServiceAccount = "other"
	if connection.ValidateRenewal() == nil {
		t.Fatal("token subject mismatch accepted")
	}
	connection.Renewal.ServiceAccount = "reader"
	connection.Credential.Token = "opaque-token"
	if connection.ValidateRenewal() == nil {
		t.Fatal("opaque credential accepted for automatic renewal")
	}
	connection.Credential = nil
	connection.Remote = false
	if connection.ValidateRenewal() == nil {
		t.Fatal("local renewal policy accepted")
	}
}
