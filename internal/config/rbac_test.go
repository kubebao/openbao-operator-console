package config

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

func TestOperatorEnrollmentAndNamedRBAC(t *testing.T) {
	connection := Connection{
		Access:     Access{Mode: AccessInCluster},
		ID:         "one",
		Name:       "One",
		Namespaces: []Namespace{{Name: "team", Environment: "production"}},
		Operators: []Operator{
			{ID: "operator", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"team"}},
		},
	}
	enrollment := Config{MaxBackupAgeHours: 24, Connections: []Connection{connection}}
	if err := enrollment.Validate(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteRBAC(&output, connection, "reader", "console"); err != nil {
		t.Fatal(err)
	}
	manifest := output.String()
	if !strings.Contains(manifest, "resourceNames:\n  - controller") || !strings.Contains(manifest, "namespace: system") ||
		strings.Contains(manifest, "secrets") ||
		strings.Contains(manifest, "ClusterRole") ||
		strings.Contains(manifest, "watch") {
		t.Fatalf("RBAC escaped named workload and namespace list permissions: %s", manifest)
	}
	enrollment.Connections[0].Operators[0].Namespaces = []string{"other"}
	if enrollment.Validate() == nil {
		t.Fatal("unenrolled operator responsibility accepted")
	}
	enrollment.Connections[0].Operators[0].Namespaces = []string{"team"}
	enrollment.Connections[0].Operators = append(
		enrollment.Connections[0].Operators,
		Operator{ID: "two", Namespace: "system", Deployment: "other", Container: "manager", Namespaces: []string{"team"}},
	)
	if enrollment.Validate() == nil {
		t.Fatal("ambiguous operator responsibility accepted")
	}
}

func controllerConnection(id, deployment string) Connection {
	return Connection{
		Access:     Access{Mode: AccessInCluster},
		ID:         id,
		Name:       id,
		Namespaces: []Namespace{{Name: "team", Environment: "other"}},
		Operators: []Operator{
			{ID: "operator", Namespace: "system", Deployment: deployment, Container: "manager", Namespaces: []string{"team"}},
		},
	}
}

// readerRoleNames writes reader RBAC and returns the manifest and its role names. It fails
// when a manifest changes the reader identity or references another connection's role.
func readerRoleNames(t *testing.T, connection Connection, account string) (string, []string) {
	t.Helper()
	var output bytes.Buffer
	if err := WriteRBAC(&output, connection, account, "console"); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, document := range strings.Split(output.String(), "---\n") {
		if document == "" {
			continue
		}
		var manifest struct {
			Kind     string `json:"kind"`
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			RoleRef struct {
				Name string `json:"name"`
			} `json:"roleRef"`
		}
		if err := yaml.Unmarshal([]byte(document), &manifest); err != nil {
			t.Fatal(err)
		}
		name := manifest.Metadata.Name
		switch {
		case manifest.Kind == "ServiceAccount" && name != account:
			t.Fatal("generated RBAC changed the reader identity")
		case manifest.Kind == "ServiceAccount":
			continue
		case len(validation.IsDNS1123Subdomain(name)) != 0 || name == account+"-reader":
			t.Fatal("role name was invalid or collided with the Helm reader role")
		case manifest.Kind == "RoleBinding" && manifest.RoleRef.Name != name:
			t.Fatal("role binding referenced a different connection's role")
		}
		names = append(names, name)
	}
	return output.String(), names
}

func TestReaderRBACNamesAreStableAndConnectionScoped(t *testing.T) {
	first := controllerConnection("first", "first-controller")
	manifest, names := readerRoleNames(t, first, "console-reader")
	if repeated, _ := readerRoleNames(t, first, "console-reader"); repeated != manifest {
		t.Fatal("same enrollment generated different role identities")
	}
	_, other := readerRoleNames(t, controllerConnection("second", "second-controller"), "console-reader")
	if slices.ContainsFunc(names, func(name string) bool { return slices.Contains(other, name) }) {
		t.Fatal("two connections sharing namespaces overwrote the same reader role")
	}
	readerRoleNames(t, first, strings.Repeat("a", 253))
}

func TestReaderRBACRejectsInvalidScopeBeforeWriting(t *testing.T) {
	for _, connection := range []Connection{
		{
			ID:         "one",
			Name:       "One",
			Access:     Access{Mode: AccessInCluster},
			Namespaces: []Namespace{{Name: "invalid/namespace", Environment: "other"}},
		},
		{
			ID:         "one",
			Name:       "One",
			Access:     Access{Mode: AccessInCluster},
			Namespaces: []Namespace{{Name: "team", Environment: "other"}},
			Operators: []Operator{
				{ID: "operator", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"unenrolled"}},
			},
		},
	} {
		var output bytes.Buffer
		if WriteRBAC(&output, connection, "reader", "console") == nil || output.Len() != 0 {
			t.Fatal("invalid scope produced reader manifests")
		}
	}
}
