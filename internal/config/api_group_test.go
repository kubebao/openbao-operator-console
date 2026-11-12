package config

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIGroupDefaultsAndSelectedReaderRBAC(t *testing.T) {
	t.Parallel()
	for _, group := range []APIGroup{"", APIGroupOpenBao, APIGroupKubeBao} {
		connection := controllerConnection("selected", "controller")
		connection.APIGroup = group
		want := string(group)
		if want == "" {
			want = string(APIGroupOpenBao)
		}
		if string(connection.EffectiveAPIGroup()) != want {
			t.Fatal("legacy enrollment default changed")
		}
		var output bytes.Buffer
		if err := WriteRBAC(&output, connection, "reader", "console"); err != nil {
			t.Fatal(err)
		}
		other := "kubebao.org"
		if want == other {
			other = "openbao.org"
		}
		if !strings.Contains(output.String(), "- "+want) || strings.Contains(output.String(), other) {
			t.Fatal("reader grants did not stay within the selected API")
		}
		for _, forbidden := range []string{"secrets", "pods", "patch", "watch", "ClusterRole", "nonResourceURLs"} {
			if strings.Contains(output.String(), forbidden) {
				t.Fatalf("API selection added %s permission", forbidden)
			}
		}
	}
}

func TestAPIGroupEnrollmentFilesAndValidation(t *testing.T) {
	t.Parallel()
	for _, group := range []string{"", "openbao.org", "kubebao.org", "other.org", "kubebao.org/v1alpha1"} {
		connection := controllerConnection("selected", "controller")
		connection.APIGroup = APIGroup(group)
		valid := group == "" || group == "openbao.org" || group == "kubebao.org"
		file := filepath.Join(t.TempDir(), "enrollment.yaml")
		data, err := json.Marshal(enrollmentFile{MaxBackupAgeHours: 24, Connections: []enrollmentConnection{{
			ID: connection.ID, Name: connection.Name, APIGroup: connection.APIGroup, Namespaces: connection.Namespaces,
		}}})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(file)
		if (err == nil) != valid {
			t.Fatalf("API group %q validation=%v", group, err)
		}
		if valid && loaded.Connections[0].APIGroup != APIGroup(group) {
			t.Fatal("enrollment file lost the selected or omitted group")
		}
		var output bytes.Buffer
		if !valid && (WriteRBAC(&output, connection, "reader", "console") == nil || output.Len() != 0) {
			t.Fatal("invalid API group wrote reader grants")
		}
	}
	legacy := controllerConnection("legacy", "controller")
	native := controllerConnection("native", "controller")
	native.APIGroup = APIGroupKubeBao
	if err := (Config{MaxBackupAgeHours: 24, Connections: []Connection{legacy, native}}).Validate(); err != nil {
		t.Fatal("mixed API enrollments rejected", err)
	}
}
