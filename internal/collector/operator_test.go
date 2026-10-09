package collector

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestOperatorObservationUsesNamedWorkloadAndCurrentGeneration(t *testing.T) {
	workload := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{"name": "controller", "namespace": "system", "uid": "controller-uid", "generation": int64(3), "labels": map[string]interface{}{"app.kubernetes.io/version": "0.5.1"}},
		"spec":     map[string]interface{}{"replicas": int64(1), "template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "manager", "image": "registry.example/operator:v0.5.1", "env": []interface{}{map[string]interface{}{"name": "PRIVATE", "value": "must-never-project"}}}}}}},
		"status":   map[string]interface{}{"observedGeneration": int64(2), "readyReplicas": int64(1), "updatedReplicas": int64(1), "conditions": []interface{}{map[string]interface{}{"type": "Available", "status": "True"}}},
	}}
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), workload)
	reader := &Reader{client: client}
	enrolled := config.Operator{ID: "operator", Namespace: "system", Deployment: "controller", Container: "manager", Namespaces: []string{"team"}}
	observation := reader.operator(context.Background(), enrolled, time.Now())
	if observation.Collection != "current" || observation.Health != "unknown" || observation.Version == nil || *observation.Version != "0.5.1" {
		t.Fatal("stale generation became a current healthy controller")
	}
	_ = unstructured.SetNestedField(workload.Object, int64(3), "status", "observedGeneration")
	client.PrependReactor("get", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetNamespace() != "system" || action.(ktesting.GetAction).GetName() != "controller" {
			t.Fatal("read escaped named enrollment")
		}
		return true, workload, nil
	})
	observation = reader.operator(context.Background(), enrolled, time.Now())
	if observation.Health != "available" || observation.Uid == nil || *observation.Uid != "controller-uid" {
		t.Fatal("current controller workload was not projected")
	}
	encoded, _ := json.Marshal(observation)
	if strings.Contains(string(encoded), "must-never-project") {
		t.Fatal("unapproved workload data escaped projection")
	}
	unstructured.RemoveNestedField(workload.Object, "status", "readyReplicas")
	unstructured.RemoveNestedField(workload.Object, "status", "updatedReplicas")
	if result := reader.operator(context.Background(), enrolled, time.Now()); result.Health != "unavailable" || result.ReadyReplicas == nil || *result.ReadyReplicas != 0 {
		t.Fatal("observed zero replicas became unknown or available")
	}
	enrolled.Container = "wrong"
	if result := reader.operator(context.Background(), enrolled, time.Now()); result.Collection != "invalid" || result.Health != "unknown" {
		t.Fatal("wrong container enrollment became a valid installation")
	}
	client.PrependReactor("get", "deployments", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(action.GetResource().GroupResource(), "controller", nil)
	})
	if result := reader.operator(context.Background(), enrolled, time.Now()); result.Collection != "denied" || result.Version != nil {
		t.Fatal("denial became an absent or observed controller")
	}
}
