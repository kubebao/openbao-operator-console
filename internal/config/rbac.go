package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

// WriteRBAC emits manifests for one enrollment. It never contacts Kubernetes.
func WriteRBAC(w io.Writer, connection Connection, serviceAccount, serviceNamespace string) error {
	if err := (Config{Connections: []Connection{connection}, MaxBackupAgeHours: 24}).Validate(); err != nil {
		return err
	}
	if len(validation.IsDNS1123Subdomain(serviceAccount)) != 0 || len(validation.IsDNS1123Label(serviceNamespace)) != 0 {
		return errors.New("reader ServiceAccount and namespace must be Kubernetes names")
	}
	digest := sha256.Sum256([]byte(connection.ID))
	suffix := "-reader-" + hex.EncodeToString(digest[:8])
	prefix := strings.TrimRight(serviceAccount[:min(len(serviceAccount), 253-len(suffix))], ".-")
	roleName := prefix + suffix
	if len(validation.IsDNS1123Subdomain(roleName)) != 0 {
		return errors.New("generated reader role name must be a Kubernetes name")
	}
	type rule struct {
		APIGroups     []string `json:"apiGroups"`
		Resources     []string `json:"resources"`
		Verbs         []string `json:"verbs"`
		ResourceNames []string `json:"resourceNames,omitempty"`
	}
	rules := make(map[string][]rule)
	if connection.Renewal != nil {
		if serviceAccount != connection.Renewal.ServiceAccount || serviceNamespace != connection.Renewal.Namespace {
			return errors.New("reader identity must match the renewal ServiceAccount")
		}
		rules[serviceNamespace] = append(
			rules[serviceNamespace],
			rule{
				APIGroups:     []string{""},
				Resources:     []string{"serviceaccounts/token"},
				Verbs:         []string{"create"},
				ResourceNames: []string{serviceAccount},
			},
		)
	}
	for _, namespace := range connection.Namespaces {
		rules[namespace.Name] = append(
			rules[namespace.Name],
			rule{
				APIGroups: []string{"openbao.org"},
				Resources: []string{"openbaoclusters", "openbaorestores"},
				Verbs:     []string{"list"},
			},
		)
	}
	names := make(map[string][]string)
	for _, operator := range connection.Operators {
		names[operator.Namespace] = append(names[operator.Namespace], operator.Deployment)
	}
	for namespace, deployments := range names {
		sort.Strings(deployments)
		rules[namespace] = append(
			rules[namespace],
			rule{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get"}, ResourceNames: deployments},
		)
	}
	emit := func(value any) error {
		data, err := yaml.Marshal(value)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "---\n%s", data)
		return err
	}
	if err := emit(
		map[string]any{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata":   map[string]string{"name": serviceAccount, "namespace": serviceNamespace},
		},
	); err != nil {
		return err
	}
	namespaces := make([]string, 0, len(rules))
	for namespace := range rules {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	for _, namespace := range namespaces {
		metadata := map[string]string{"name": roleName, "namespace": namespace}
		if err := emit(
			map[string]any{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "Role",
				"metadata":   metadata,
				"rules":      rules[namespace],
			},
		); err != nil {
			return err
		}
		if err := emit(map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": metadata,
			"subjects": []map[string]string{{"kind": "ServiceAccount", "name": serviceAccount, "namespace": serviceNamespace}},
			"roleRef":  map[string]string{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": roleName},
		}); err != nil {
			return err
		}
	}
	return nil
}
