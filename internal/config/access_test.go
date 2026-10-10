package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAccessRequiresFieldsThatMatchItsMode(t *testing.T) {
	credential := &Credential{Server: "https://api.example", Token: "reader"}
	for _, test := range []struct {
		name   string
		access Access
		valid  bool
	}{
		{"in-cluster", Access{Mode: AccessInCluster}, true},
		{"in-cluster with kubeconfig", Access{Mode: AccessInCluster, Kubeconfig: "/reader"}, false},
		{"in-cluster with credential", Access{Mode: AccessInCluster, Credential: credential}, false},
		{"kubeconfig path", Access{Mode: AccessKubeconfig, Kubeconfig: "/reader"}, true},
		{"kubeconfig context only", Access{Mode: AccessKubeconfig, Context: "reader"}, true},
		{"kubeconfig without source", Access{Mode: AccessKubeconfig}, false},
		{"kubeconfig with credential", Access{Mode: AccessKubeconfig, Kubeconfig: "/reader", Credential: credential}, false},
		{"remote", Access{Mode: AccessRemote, Credential: credential}, true},
		{"remote before credential issuance", Access{Mode: AccessRemote}, true},
		{"remote with invalid credential", Access{Mode: AccessRemote, Credential: &Credential{Server: "http://api.example"}}, false},
		{"remote with kubeconfig", Access{Mode: AccessRemote, Kubeconfig: "/reader"}, false},
		{"missing mode", Access{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.access.Validate(); (err == nil) != test.valid {
				t.Fatalf("Validate() = %v, want valid %t", err, test.valid)
			}
		})
	}
}

func TestEnrollmentFileInfersAccessFromKubeconfigFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "enrollment.yaml")
	file := `maxBackupAgeHours: 24
connections:
  - id: local
    name: Local
    namespaces: [{name: team, environment: development}]
  - id: remote
    name: Remote
    kubeconfig: /etc/console/reader.yaml
    context: reader
    namespaces: [{name: team, environment: production}]
users: []
relationships: []
`
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	enrollment, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := enrollment.Connections[0].Access; got != (Access{Mode: AccessInCluster}) {
		t.Fatalf("connection without kubeconfig = %+v", got)
	}
	want := Access{Mode: AccessKubeconfig, Kubeconfig: "/etc/console/reader.yaml", Context: "reader"}
	if got := enrollment.Connections[1].Access; got != want {
		t.Fatalf("connection with kubeconfig = %+v", got)
	}
	inline := strings.Replace(
		file,
		"    context: reader\n",
		"    context: reader\n    credential: {server: https://api.example, token: reader}\n",
		1,
	)
	if err := os.WriteFile(path, []byte(inline), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("Load() = %v, want an unknown credential field error", err)
	}
}
