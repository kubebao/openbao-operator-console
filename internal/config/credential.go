package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/url"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// Credential contains embedded authentication material, never file paths or plugins.
type Credential struct {
	Server      string `json:"server"`
	CA          string `json:"ca"`
	Token       string `json:"token"`
	Certificate string `json:"certificate"`
	Key         string `json:"key"`
}

func (c Credential) Validate() error {
	endpoint, err := url.Parse(c.Server)
	if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" || endpoint.User != nil || endpoint.RawQuery != "" ||
		endpoint.Fragment != "" {
		return errors.New("API server must be an HTTPS URL without credentials, query, or fragment")
	}
	if c.CA != "" && !x509.NewCertPool().AppendCertsFromPEM([]byte(c.CA)) {
		return errors.New("CA must contain PEM certificates")
	}
	if (c.Certificate == "") != (c.Key == "") {
		return errors.New("client certificate and key must be supplied together")
	}
	if c.Certificate != "" {
		if _, err := tls.X509KeyPair([]byte(c.Certificate), []byte(c.Key)); err != nil {
			return errors.New("client certificate and key are invalid")
		}
	}
	if (c.Token == "") == (c.Certificate == "") {
		return errors.New("supply either a bearer token or a client certificate")
	}
	if strings.ContainsAny(c.Token, "\r\n") {
		return errors.New("bearer token must not contain line breaks")
	}
	return nil
}

func ImportKubeconfig(data []byte, context string) (Credential, error) {
	source, err := clientcmd.Load(data)
	if err != nil {
		return Credential{}, errors.New("kubeconfig could not be decoded")
	}
	// Reject every entry, including unused contexts, before selecting a credential.
	for _, cluster := range source.Clusters {
		if cluster == nil {
			return Credential{}, errors.New("kubeconfig cluster is missing")
		}
		if cluster.CertificateAuthority != "" || cluster.InsecureSkipTLSVerify || cluster.ProxyURL != "" || cluster.TLSServerName != "" {
			return Credential{}, errors.New(
				"kubeconfig must embed its CA and must not disable TLS, override the server name, or configure a proxy",
			)
		}
	}
	for _, user := range source.AuthInfos {
		if user == nil {
			return Credential{}, errors.New("kubeconfig reader is missing")
		}
		if user.Exec != nil || user.AuthProvider != nil || user.TokenFile != "" || user.ClientCertificate != "" || user.ClientKey != "" ||
			user.Username != "" ||
			user.Password != "" ||
			user.Impersonate != "" ||
			len(user.ImpersonateGroups) != 0 ||
			len(user.ImpersonateUserExtra) != 0 ||
			user.ImpersonateUID != "" {
			return Credential{}, errors.New(
				"kubeconfig must use an embedded token or certificate without plugins, file references, or impersonation",
			)
		}
	}
	if context == "" {
		context = source.CurrentContext
	}
	selected := source.Contexts[context]
	if selected == nil {
		return Credential{}, errors.New("select an existing kubeconfig context")
	}
	cluster, user := source.Clusters[selected.Cluster], source.AuthInfos[selected.AuthInfo]
	if cluster == nil || user == nil {
		return Credential{}, errors.New("kubeconfig context has no cluster or reader identity")
	}
	result := Credential{
		Server:      cluster.Server,
		CA:          string(cluster.CertificateAuthorityData),
		Token:       user.Token,
		Certificate: string(user.ClientCertificateData),
		Key:         string(user.ClientKeyData),
	}
	return result, result.Validate()
}

// Resolve maps a verified principal to its current access. User and group bindings
// belong to AccessIssuer, so a principal from any other issuer matches none of them.
// Local principals match only local accounts.
func (c Config) Resolve(principal Principal) (User, bool) {
	var result User
	var enrolled bool
	if principal.Issuer == LocalIssuer {
		result, enrolled = c.resolveLocal(principal.Subject)
	} else {
		result, enrolled = c.resolveFederated(principal)
	}
	if !enrolled {
		return result, false
	}
	return c.effectiveAccess(result), true
}

// resolveFederated matches an OIDC principal against user and group bindings.
func (c Config) resolveFederated(principal Principal) (User, bool) {
	subject, memberships := principal.Subject, principal.Groups
	result := User{Subject: subject, Name: subject, Scopes: []Scope{}}
	if principal.Issuer == "" || principal.Issuer != c.AccessIssuer {
		return result, false
	}
	enrolled := false
	for _, user := range c.Users {
		if user.Subject == subject {
			result = user
			result.Scopes = append([]Scope{}, user.Scopes...)
			enrolled = true
			break
		}
	}
	for _, binding := range c.Groups {
		for _, membership := range memberships {
			if membership == binding.Name {
				enrolled = true
				result.Admin = result.Admin || binding.Admin
				result.Scopes = append(result.Scopes, binding.Scopes...)
				break
			}
		}
	}
	if result.Name == "" {
		result.Name = subject
	}
	return result, enrolled
}

// effectiveAccess gives administrators every enrolled namespace and intersects other
// scopes with current enrollment.
func (c Config) effectiveAccess(result User) User {
	if result.Admin {
		result.Scopes = []Scope{}
		for _, connection := range c.Connections {
			namespaces := []string{}
			for _, ns := range connection.Namespaces {
				namespaces = append(namespaces, ns.Name)
			}
			result.Scopes = append(result.Scopes, Scope{ConnectionID: connection.ID, Namespaces: namespaces})
		}
	}
	// Enrollment is intersected on every request, including old sessions and disabled readers.
	active := map[string]map[string]bool{}
	for _, connection := range c.Connections {
		active[connection.ID] = map[string]bool{}
		for _, ns := range connection.Namespaces {
			active[connection.ID][ns.Name] = true
		}
	}
	scopes := []Scope{}
	for _, scope := range result.Scopes {
		namespaces := []string{}
		for _, ns := range scope.Namespaces {
			if active[scope.ConnectionID][ns] {
				namespaces = append(namespaces, ns)
			}
		}
		if len(namespaces) > 0 {
			scopes = append(scopes, Scope{ConnectionID: scope.ConnectionID, Namespaces: namespaces})
		}
	}
	result.Scopes = scopes
	return result
}
