package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// TokenRenewal limits token issuance to one administrator-declared ServiceAccount.
type TokenRenewal struct {
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
}

func (c Connection) ValidateRenewal() error {
	if c.Renewal == nil {
		return nil
	}
	if !c.Remote && c.Credential == nil && c.Kubeconfig == "" {
		return fmt.Errorf("automatic renewal requires a remote connection")
	}
	if len(validation.IsDNS1123Label(c.Renewal.Namespace)) != 0 || len(validation.IsDNS1123Subdomain(c.Renewal.ServiceAccount)) != 0 {
		return fmt.Errorf("renewal ServiceAccount and namespace must be Kubernetes names")
	}
	if c.Credential != nil {
		if c.Credential.Token == "" || c.Credential.Certificate != "" {
			return fmt.Errorf("automatic renewal requires a ServiceAccount bearer token")
		}
		// This declaration selects the supported credential mode; it does not
		// authenticate the token. Kubernetes authorizes every TokenRequest.
		parts := strings.Split(c.Credential.Token, ".")
		if len(parts) != 3 || len(c.Credential.Token) > 16384 {
			return fmt.Errorf("automatic renewal requires a ServiceAccount JWT")
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return fmt.Errorf("automatic renewal requires a ServiceAccount JWT")
		}
		var claims struct {
			Subject string `json:"sub"`
		}
		if json.Unmarshal(payload, &claims) != nil || claims.Subject != "system:serviceaccount:"+c.Renewal.Namespace+":"+c.Renewal.ServiceAccount {
			return fmt.Errorf("token subject must match the renewal ServiceAccount")
		}
	}
	return nil
}
