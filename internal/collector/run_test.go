package collector

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

type recordingRepository struct{ applied chan string }

func (r recordingRepository) Apply(_ context.Context, batch store.Batch) error {
	r.applied <- batch.Connection.Id
	return nil
}
func (recordingRepository) Snapshot(context.Context) (contract.Snapshot, error) {
	return contract.Snapshot{}, nil
}
func (recordingRepository) History(context.Context, string) ([]contract.Observation, error) {
	return nil, nil
}
func (recordingRepository) Close() {}

func TestSlowConnectionDoesNotDelayAnotherConnection(t *testing.T) {
	blocked := make(chan struct{})
	reader := func(id string, slow bool) *Reader {
		client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
			{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaoclusters"}:   "OpenBaoClusterList",
			{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorestores"}:   "OpenBaoRestoreList",
			{Group: "openbao.org", Version: "v1alpha1", Resource: "openbaorecoveries"}: "OpenBaoRecoveryList",
		})
		client.PrependReactor("list", "*", func(action ktesting.Action) (bool, runtime.Object, error) {
			if slow && action.GetResource().Resource == "openbaoclusters" {
				<-blocked
			}
			return true, &unstructured.UnstructuredList{}, nil
		})
		return &Reader{client: client, connection: config.Connection{ID: id, Namespaces: []config.Namespace{{Name: "team"}}}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repository := recordingRepository{applied: make(chan string, 2)}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		collectRound(ctx, []*Reader{reader("slow", true), reader("fast", false)}, repository, slog.New(slog.NewTextHandler(io.Discard, nil)))
	}()
	select {
	case id := <-repository.applied:
		close(blocked)
		if id != "fast" {
			t.Fatal("blocked connector persisted before fast connector")
		}
	case <-ctx.Done():
		close(blocked)
		t.Fatal("slow connector delayed the fleet")
	}
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal("collection workers did not finish")
	}
}
