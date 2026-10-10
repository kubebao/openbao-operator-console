package config

import (
	"errors"

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
	if c.Access.Mode == AccessInCluster {
		return errors.New("automatic renewal requires a remote connection")
	}
	if len(validation.IsDNS1123Label(c.Renewal.Namespace)) != 0 || len(validation.IsDNS1123Subdomain(c.Renewal.ServiceAccount)) != 0 {
		return errors.New("renewal ServiceAccount and namespace must be Kubernetes names")
	}
	if credential := c.Access.Credential; credential != nil {
		if credential.Token == "" || credential.Certificate != "" {
			return errors.New("automatic renewal requires a ServiceAccount bearer token")
		}
		// This declaration selects the supported credential mode; it does not
		// authenticate the token. Kubernetes authorizes every TokenRequest.
		var claims struct {
			Subject string `json:"sub"`
		}
		if err := tokenClaims(credential.Token, &claims); err != nil {
			return errors.New("automatic renewal requires a ServiceAccount JWT")
		}
		if claims.Subject != "system:serviceaccount:"+c.Renewal.Namespace+":"+c.Renewal.ServiceAccount {
			return errors.New("token subject must match the renewal ServiceAccount")
		}
	}
	return nil
}
