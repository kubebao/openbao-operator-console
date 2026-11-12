package config

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"sigs.k8s.io/yaml"
)

type chartReaderRule struct {
	APIGroups     []string `json:"apiGroups"`
	Resources     []string `json:"resources"`
	Verbs         []string `json:"verbs"`
	ResourceNames []string `json:"resourceNames"`
}

type chartReaderManifest struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Rules    []chartReaderRule `json:"rules"`
	Subjects []struct {
		Kind      string `json:"kind"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"subjects"`
	RoleRef struct {
		APIGroup string `json:"apiGroup"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	} `json:"roleRef"`
}

func renderReaderChart(t *testing.T, values map[string]any, invalid bool) []chartReaderManifest {
	t.Helper()
	if os.Getenv("CONSOLE_TEST_HELM") != "1" {
		t.Skip("make helm-check enables rendered reader-RBAC qualification")
	}
	values["existingSecret"] = "console-runtime"
	values["publicURL"] = "https://console.example.com"
	values["fullnameOverride"] = "reader-fixture"
	values["serviceAccount"] = map[string]any{"name": "console-reader"}
	file := filepath.Join(t.TempDir(), "values.json")
	data, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "helm", "template", "console", "../../charts/openbao-operator-console",
		"--namespace", "console-system", "--values", file)
	output, err := command.CombinedOutput()
	if invalid {
		if err == nil || !strings.Contains(string(output), "values don't meet the specifications") {
			t.Fatal("reader-scope schema did not reject the invalid values")
		}
		return nil
	}
	if err != nil {
		t.Fatalf("render reader RBAC: %v", err)
	}
	result := []chartReaderManifest{}
	for _, document := range strings.Split(string(output), "\n---") {
		var manifest chartReaderManifest
		if err := yaml.Unmarshal([]byte(document), &manifest); err != nil {
			t.Fatal("decode rendered chart")
		}
		if manifest.Kind == "ClusterRole" || manifest.Kind == "ClusterRoleBinding" {
			t.Fatal("reader scopes created cluster-wide authorization")
		}
		if manifest.Kind == "Role" || manifest.Kind == "RoleBinding" {
			result = append(result, manifest)
		}
	}
	return result
}

func checkChartBinding(t *testing.T, manifest chartReaderManifest, role string) {
	t.Helper()
	if len(manifest.Subjects) != 1 || manifest.Subjects[0].Kind != "ServiceAccount" ||
		manifest.Subjects[0].Name != "console-reader" || manifest.Subjects[0].Namespace != "console-system" ||
		manifest.RoleRef.APIGroup != "rbac.authorization.k8s.io" || manifest.RoleRef.Kind != "Role" || manifest.RoleRef.Name != role {
		t.Fatal("reader RoleBinding changed its scoped reader identity")
	}
}

func checkChartOperator(t *testing.T, manifest chartReaderManifest) {
	t.Helper()
	if manifest.Metadata.Namespace != "operators" {
		t.Fatal("named controller scope changed")
	}
	if manifest.Kind == "RoleBinding" {
		checkChartBinding(t, manifest, "reader-fixture-operator-reader")
		return
	}
	want := []chartReaderRule{{
		APIGroups: []string{"apps"}, Resources: []string{"deployments"},
		Verbs: []string{"get"}, ResourceNames: []string{"controller"},
	}}
	if !reflect.DeepEqual(manifest.Rules, want) {
		t.Fatal("reader API groups changed named controller permissions")
	}
}

func checkChartReaderManifests(t *testing.T, manifests []chartReaderManifest, expected map[string][]string) {
	t.Helper()
	seen := map[string]map[string]bool{"Role": {}, "RoleBinding": {}}
	operators := map[string]bool{}
	for _, manifest := range manifests {
		if manifest.Metadata.Name == "reader-fixture-operator-reader" {
			if operators[manifest.Kind] {
				t.Fatal("named controller RBAC was duplicated")
			}
			operators[manifest.Kind] = true
			checkChartOperator(t, manifest)
			continue
		}
		groups, enrolled := expected[manifest.Metadata.Namespace]
		if !enrolled || manifest.Metadata.Name != "reader-fixture-reader" || seen[manifest.Kind][manifest.Metadata.Namespace] {
			t.Fatal("reader RBAC duplicated or escaped its declared namespace and existing identity")
		}
		seen[manifest.Kind][manifest.Metadata.Namespace] = true
		if manifest.Kind == "RoleBinding" {
			checkChartBinding(t, manifest, "reader-fixture-reader")
			continue
		}
		want := []chartReaderRule{{
			APIGroups: groups, Resources: []string{"openbaoclusters", "openbaorestores"}, Verbs: []string{"list"},
		}}
		if !reflect.DeepEqual(manifest.Rules, want) {
			t.Fatal("reader Role added undeclared API groups, resources, or verbs")
		}
	}
	if len(seen["Role"]) != len(expected) || len(seen["RoleBinding"]) != len(expected) {
		t.Fatal("not every declared namespace received exactly one reader Role and RoleBinding")
	}
	if len(operators) != 2 {
		t.Fatal("named controller Role or RoleBinding is missing")
	}
}

func TestHelmReaderScopesStayWithinDeclaredPermissions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values map[string]any
		groups map[string][]string
	}{
		{
			name:   "legacy",
			values: map[string]any{"readerNamespaces": []string{"legacy"}},
			groups: map[string][]string{"legacy": {"openbao.org"}},
		},
		{
			name:   "native",
			values: map[string]any{"readerScopes": []map[string]string{{"namespace": "native", "apiGroup": "kubebao.org"}}},
			groups: map[string][]string{"native": {"kubebao.org"}},
		},
		{
			name: "mixed",
			values: map[string]any{
				"readerNamespaces": []string{"shared", "legacy", "shared"},
				"readerScopes": []map[string]string{
					{"namespace": "shared", "apiGroup": "kubebao.org"},
					{"namespace": "shared", "apiGroup": "openbao.org"},
					{"namespace": "native", "apiGroup": "kubebao.org"},
				},
			},
			groups: map[string][]string{
				"legacy": {"openbao.org"}, "native": {"kubebao.org"}, "shared": {"kubebao.org", "openbao.org"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.values["operatorReaders"] = []map[string]any{{"namespace": "operators", "deployments": []string{"controller"}}}
			checkChartReaderManifests(t, renderReaderChart(t, tc.values, false), tc.groups)
		})
	}
}

func TestHelmReaderScopesRejectInvalidValues(t *testing.T) {
	for _, scopes := range []any{
		[]map[string]string{{"namespace": "team", "apiGroup": "other.org"}},
		[]map[string]string{{"namespace": "team"}},
		[]map[string]string{{"apiGroup": "kubebao.org"}},
		[]map[string]string{{"namespace": "team", "apiGroup": "kubebao.org"}, {"namespace": "team", "apiGroup": "kubebao.org"}},
	} {
		renderReaderChart(t, map[string]any{"readerScopes": scopes}, true)
	}
}
