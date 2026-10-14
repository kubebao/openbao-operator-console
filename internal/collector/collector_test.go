package collector

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
)

func TestCollectorPaginationAndOptionalAPIs(t *testing.T) {
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaoclusters"}:   "OpenBaoClusterList",
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorestores"}:   "OpenBaoRestoreList",
		{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorecoveries"}: "OpenBaoRecoveryList",
	})
	pages := 0
	client.PrependReactor("list", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetResource().Resource != "openbaoclusters" {
			return true, nil, apierrors.NewNotFound(action.GetResource().GroupResource(), "optional")
		}
		pages++
		page := &unstructured.UnstructuredList{}
		page.Items = []unstructured.Unstructured{
			{Object: map[string]any{"metadata": map[string]any{"name": "first", "namespace": "team", "uid": "first-uid"}}},
		}
		if pages == 1 {
			page.SetContinue("next")
		} else {
			page.Items[0].SetName("second")
			page.Items[0].SetUID("second-uid")
		}
		return true, page, nil
	})
	reader := &Reader{
		client:     client,
		connection: config.Connection{ID: "one", Namespaces: []config.Namespace{{Name: "team", Environment: "production"}}},
	}
	batch := reader.Collect(context.Background(), time.Now())
	if pages != 2 || len(batch.Deployments) != 2 || batch.Connection.Collection != "current" {
		t.Fatal("pagination or optional CRD handling lost inventory")
	}
	if len(batch.Connection.Capabilities) != 3 || batch.Connection.Capabilities[0].State != "available" ||
		batch.Connection.Capabilities[1].State != "unsupported" {
		t.Fatal("API absence was not reported separately from a successful list")
	}
	client.PrependReactor("list", "openbaorestores", func(action ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(action.GetResource().GroupResource(), "team", nil)
	})
	batch = reader.Collect(context.Background(), time.Now())
	if batch.Connection.Collection != "current" || len(batch.Deployments) == 0 || batch.Deployments[0].Recovery != nil {
		t.Fatal("optional permission denial blocked workload visibility or fabricated recovery evidence")
	}
	if batch.Connection.Capabilities[1].State != "denied" || batch.Connection.Capabilities[2].State != "unsupported" {
		t.Fatal("optional API permission denial or absence was concealed")
	}
}

func TestRecoveryProjectionKeepsSubmissionSeparateFromApplication(t *testing.T) {
	request := unstructured.Unstructured{
		Object: map[string]any{
			"status": map[string]any{
				"phase":          "Unknown",
				"execution":      map[string]any{"stage": "RestoreSubmitted"},
				"completionTime": metav1.Now().Format(time.RFC3339),
			},
		},
	}
	result := ProjectRecovery(request, "snapshot", time.Now(), 10*time.Minute)
	if result.Phase != "unknown" || result.Stage != "RestoreSubmitted" || result.ApplicationConfirmedAt != nil {
		t.Fatal("submission became confirmed restore application")
	}
}
