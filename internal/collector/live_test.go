package collector

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
)

// This test reads only the explicitly named disposable development namespace.
func TestKindOperatorProjection(t *testing.T) {
	path := os.Getenv("CONSOLE_TEST_KUBECONFIG")
	if path == "" {
		t.Skip("set CONSOLE_TEST_KUBECONFIG to the isolated console-dev cluster")
	}
	reader, err := New(
		config.Connection{
			ID:         "kind-console-dev",
			Name:       "Console development",
			Kubeconfig: path,
			Context:    "kind-console-dev",
			Namespaces: []config.Namespace{{Name: "console-sample", Team: "Platform", Environment: "development", Location: "Local"}},
			Operators: []config.Operator{
				{
					ID:         "operator",
					Namespace:  "console-system",
					Deployment: "console-operator-openbao-operator-controller",
					Container:  "manager",
					Namespaces: []string{"console-sample"},
				},
			},
		},
		evidence.Policy{MaxBackupAge: 24 * time.Hour, StartupGrace: 10 * time.Minute},
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	batch := reader.Collect(ctx, time.Now().UTC())
	if batch.Connection.Collection != "current" {
		t.Fatalf("collection = %s", batch.Connection.Collection)
	}
	if len(batch.Connection.Operators) != 1 || batch.Connection.Operators[0].Collection != "current" ||
		batch.Connection.Operators[0].Uid == nil ||
		batch.Connection.Operators[0].Image == nil {
		t.Fatal("enrolled controller Deployment could not be observed")
	}
	for _, d := range batch.Deployments {
		if d.Name != "console-bao" {
			continue
		}
		if d.Uid == "" || d.DesiredVersion != "2.7.0" || d.DesiredReplicas == nil || *d.DesiredReplicas != 1 || d.ResourceVersion == "" ||
			len(d.Conditions) == 0 ||
			d.Evidence.Collection != "current" ||
			d.OperatorId == nil ||
			*d.OperatorId != "operator" {
			t.Fatalf("unexpected real operator projection: %#v", d)
		}
		return
	}
	t.Fatal("isolated operator deployment was not observed")
}
