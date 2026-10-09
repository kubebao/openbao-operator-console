package config

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

func TestOperatorEnrollmentAndNamedRBAC(t *testing.T) {
	connection := Connection{
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

func TestReaderRBACNamesAreStableAndConnectionScoped(t *testing.T) {
	first := Connection{
		ID:         "first",
		Name:       "First",
		Namespaces: []Namespace{{Name: "team", Environment: "other"}},
		Operators: []Operator{
			{ID: "operator", Namespace: "system", Deployment: "first-controller", Container: "manager", Namespaces: []string{"team"}},
		},
	}
	second := Connection{
		ID:         "second",
		Name:       "Second",
		Namespaces: []Namespace{{Name: "team", Environment: "other"}},
		Operators: []Operator{
			{ID: "operator", Namespace: "system", Deployment: "second-controller", Container: "manager", Namespaces: []string{"team"}},
		},
	}
	roleNames := func(connection Connection, account string) (string, []string) {
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
			if manifest.Kind == "ServiceAccount" {
				if manifest.Metadata.Name != account {
					t.Fatal("generated RBAC changed the reader identity")
				}
				continue
			}
			if len(validation.IsDNS1123Subdomain(manifest.Metadata.Name)) != 0 || manifest.Metadata.Name == account+"-reader" {
				t.Fatal("role name was invalid or collided with the Helm reader role")
			}
			if manifest.Kind == "RoleBinding" && manifest.RoleRef.Name != manifest.Metadata.Name {
				t.Fatal("role binding referenced a different connection's role")
			}
			names = append(names, manifest.Metadata.Name)
		}
		return output.String(), names
	}
	manifest, names := roleNames(first, "console-reader")
	if repeated, _ := roleNames(first, "console-reader"); repeated != manifest {
		t.Fatal("same enrollment generated different role identities")
	}
	_, other := roleNames(second, "console-reader")
	for _, name := range names {
		for _, different := range other {
			if name == different {
				t.Fatal("two connections sharing namespaces overwrote the same reader role")
			}
		}
	}
	roleNames(first, strings.Repeat("a", 253))
}

func TestReaderRBACRejectsInvalidScopeBeforeWriting(t *testing.T) {
	for _, connection := range []Connection{
		{ID: "one", Name: "One", Namespaces: []Namespace{{Name: "invalid/namespace", Environment: "other"}}},
		{ID: "one", Name: "One", Namespaces: []Namespace{{Name: "team", Environment: "other"}}, Operators: []Operator{
			{ID: "operator", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"unenrolled"}},
		}},
	} {
		var output bytes.Buffer
		if WriteRBAC(&output, connection, "reader", "console") == nil || output.Len() != 0 {
			t.Fatal("invalid scope produced reader manifests")
		}
	}
}
