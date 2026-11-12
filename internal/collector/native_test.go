package collector

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func writeGroupList(w http.ResponseWriter, group config.APIGroup, resource string, items []any) {
	kind := "OpenBaoClusterList"
	if resource == "openbaorestores" {
		kind = "OpenBaoRestoreList"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"apiVersion": string(group) + "/v1alpha1", "kind": kind,
		"metadata": map[string]any{"resourceVersion": "100"}, "items": items,
	})
}

func groupCluster(group config.APIGroup) map[string]any {
	cluster := testCluster()
	cluster["apiVersion"] = string(group) + "/v1alpha1"
	cluster["metadata"].(map[string]any)["annotations"] = map[string]any{"openbao.org/restore-origin": "restore-uid"}
	return cluster
}

func groupRestore(group config.APIGroup) map[string]any {
	return map[string]any{
		"apiVersion": string(group) + "/v1alpha1", "kind": "OpenBaoRestore",
		"metadata": map[string]any{
			"name": "restore", "namespace": "team", "uid": "restore-uid",
			"creationTimestamp": time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
		},
		"spec": map[string]any{"cluster": "bao"},
		"status": map[string]any{
			"phase": "Completed", "target": map[string]any{"uid": "original"},
			"execution":  map[string]any{"targetUID": "original", "stage": "FollowThroughComplete"},
			"conditions": []any{map[string]any{"type": "Ready", "status": "True", "message": "PRIVATE_PROVIDER_CANARY"}},
		},
	}
}

func groupReader(t *testing.T, server *httptest.Server, id string, group config.APIGroup, operators ...config.Operator) *Reader {
	t.Helper()
	reader, err := New(config.Connection{
		ID: id, APIGroup: group, Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
		Operators: operators,
		Access: config.Access{Mode: config.AccessRemote, Credential: &config.Credential{
			Server: server.URL, Token: "reader",
			CA: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
		}},
	}, fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestNativeAndLegacyReadersKeepExplicitScopesAndAnalyzeRestores(t *testing.T) {
	t.Parallel()
	paths := []string{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer reader" || r.Header.Get("User-Agent") == "" {
			t.Error("collection changed method, credentials, or user agent")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/apis/apps/v1/namespaces/system/deployments/controller" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"apiVersion":"apps/v1","kind":"Deployment",
			"metadata":{"name":"controller","namespace":"system","uid":"controller","generation":1},
			"spec":{"replicas":1,"template":{"spec":{"containers":[{"name":"manager","image":"registry.example/operator:test"}]}}},
			"status":{"observedGeneration":1,"readyReplicas":1,"updatedReplicas":1,"conditions":[{"type":"Available","status":"True"}]}}`))
			return
		}
		group := config.APIGroupKubeBao
		if strings.HasPrefix(r.URL.Path, "/apis/openbao.org/") {
			group = config.APIGroupOpenBao
		}
		prefix := "/apis/" + string(group) + "/v1alpha1/namespaces/team/"
		resource := strings.TrimPrefix(r.URL.Path, prefix)
		if r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("labelSelector") != "" {
			t.Error("read changed namespace collection limits or selector")
		}
		switch resource {
		case "openbaoclusters":
			writeGroupList(w, group, resource, []any{groupCluster(group)})
		case "openbaorestores":
			writeGroupList(w, group, resource, []any{groupRestore(group)})
		default:
			t.Errorf("read escaped declared enrollment scope: %s", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	operator := config.Operator{
		ID:         "operator",
		Namespace:  "system",
		Deployment: "controller",
		Container:  "manager",
		Namespaces: []string{"team"},
	}
	ids := []string{}
	for _, group := range []config.APIGroup{config.APIGroupOpenBao, config.APIGroupKubeBao} {
		reader := groupReader(t, server, string(group), group, operator)
		start := len(paths)
		batch, diagnostic := reader.Check(context.Background(), time.Now().UTC())
		if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || diagnostic.State != DiagnosticAvailable ||
			len(batch.Deployments) != 1 {
			t.Fatalf("selected group %s could not complete collection and shared analysis", group)
		}
		deployment := batch.Deployments[0]
		if deployment.Recovery == nil || deployment.Recovery.RequestName != "restore" ||
			deployment.Recovery.Phase != fleet.RecoveryPhaseCompleted || deployment.Role != fleet.RoleRecoveryTarget ||
			deployment.OperatorID == nil || *deployment.OperatorID != "operator" {
			t.Fatalf("selected group %s lost its UID-bound recovery or enrolled controller", group)
		}
		ids = append(ids, deployment.ID)
		encoded, err := json.Marshal(batch)
		if err != nil || strings.Contains(string(encoded), "PRIVATE_PROVIDER_CANARY") {
			t.Fatal("unapproved provider data entered the collected batch")
		}
		prefix := "/apis/" + string(group) + "/v1alpha1/namespaces/team/"
		want := []string{
			"/apis/apps/v1/namespaces/system/deployments/controller",
			prefix + "openbaoclusters", prefix + "openbaorestores", prefix + "openbaoclusters",
		}
		if !reflect.DeepEqual(paths[start:], want) {
			t.Fatalf("selected group %s used unexpected collection scope: %v", group, paths[start:])
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("distinct enrollments with matching resource names and UIDs shared an identity")
	}
}

func TestNativeReaderNeverFallsBackAfterDeniedMissingOrWrongGroupEvidence(t *testing.T) {
	t.Parallel()
	for _, result := range []string{"denied", "missing", "legacy-envelope", "legacy-object"} {
		t.Run(result, func(t *testing.T) {
			calls := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/apis/kubebao.org/v1alpha1/namespaces/team/openbaoclusters" {
					t.Errorf("native reader tried another scope: %s %s", r.Method, r.URL.Path)
				}
				switch result {
				case "denied":
					w.WriteHeader(http.StatusForbidden)
				case "missing":
					w.WriteHeader(http.StatusNotFound)
				case "legacy-envelope":
					writeGroupList(w, config.APIGroupOpenBao, "openbaoclusters", []any{groupCluster(config.APIGroupOpenBao)})
				case "legacy-object":
					writeGroupList(w, config.APIGroupKubeBao, "openbaoclusters", []any{groupCluster(config.APIGroupOpenBao)})
				}
			}))
			defer server.Close()
			reader := groupReader(t, server, "native", config.APIGroupKubeBao)
			batch, diagnostic := reader.Check(context.Background(), time.Now().UTC())
			if calls != 1 || batch.Connection.Collection == fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 0 {
				t.Fatal("incomplete native evidence triggered fallback or became a current deployment")
			}
			if result == "denied" &&
				(batch.Connection.Collection != fleet.ConnectionCollectionDenied || diagnostic.State != DiagnosticForbidden) {
				t.Fatal("native denial lost its classification")
			}
			if result == "missing" &&
				(batch.Connection.Collection != fleet.ConnectionCollectionUnsupported || diagnostic.State != DiagnosticUnsupported) {
				t.Fatal("missing native API lost its classification")
			}
		})
	}
}

func TestNativeOptionalRestoreFailuresRemainIncomplete(t *testing.T) {
	t.Parallel()
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/apis/kubebao.org/v1alpha1/namespaces/team/") {
				t.Errorf("native reader escaped its declared scope: %s %s", r.Method, r.URL.Path)
			}
			if strings.HasSuffix(r.URL.Path, "/openbaorestores") {
				w.WriteHeader(code)
				return
			}
			writeGroupList(w, config.APIGroupKubeBao, "openbaoclusters", []any{groupCluster(config.APIGroupKubeBao)})
		}))
		reader := groupReader(t, server, "native", config.APIGroupKubeBao)
		batch := reader.Collect(context.Background(), time.Now().UTC())
		server.Close()
		if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 1 ||
			!batch.Deployments[0].Evidence.AnalysisIncomplete || batch.Deployments[0].Recovery != nil {
			t.Fatal("unreadable native restore evidence became complete analysis or recovery proof")
		}
	}
}

func TestNativePartialClusterListDoesNotReplaceInventory(t *testing.T) {
	t.Parallel()
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.URL.Path != "/apis/kubebao.org/v1alpha1/namespaces/team/openbaoclusters" {
			t.Errorf("partial native collection escaped its declared list: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("continue") == "next" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": "kubebao.org/v1alpha1", "kind": "OpenBaoClusterList",
			"metadata": map[string]any{"resourceVersion": "100", "continue": "next"},
			"items":    []any{groupCluster(config.APIGroupKubeBao)},
		})
	}))
	defer server.Close()
	batch := groupReader(t, server, "native", config.APIGroupKubeBao).Collect(context.Background(), time.Now().UTC())
	if calls != 2 || batch.Connection.Collection != fleet.ConnectionCollectionDenied || batch.Deployments != nil {
		t.Fatal("partially collected native inventory became a complete replacement")
	}
}

func TestNativePolicyAndRestorePresentationRejectOtherContracts(t *testing.T) {
	t.Parallel()
	cluster := normalizedCluster(t, testCluster())
	cluster.Reference.Identity.Group = "kubebao.org"
	cluster.Reference.APIVersion = "kubebao.org/v1alpha1"
	cluster.AdapterContract = string(adapter.KubeBaoV1Alpha1Snapshot)
	request := normalizedRestore(t, "restore", "original", "Completed")
	request.Reference.Identity.Group = "kubebao.org"
	request.Reference.APIVersion = "kubebao.org/v1alpha1"
	request.AdapterContract = string(adapter.KubeBaoV1Alpha1Snapshot)
	policy := analysisPolicy(time.Now().UTC(), fleet.Policy{}, adapter.KubeBaoV1Alpha1Snapshot)
	if !reflect.DeepEqual(policy.ApprovedContracts, []string{string(adapter.KubeBaoV1Alpha1Snapshot)}) ||
		!reflect.DeepEqual(policy.RestoreUIDBindingContracts, []string{string(adapter.KubeBaoV1Alpha1Snapshot)}) {
		t.Fatal("native reader diagnostic policy does not approve exactly its inspected contract")
	}
	for _, change := range []string{"valid", "legacy-request", "unqualified", "spoofed", "other-version"} {
		target, restore := cluster, request
		switch change {
		case "legacy-request":
			restore.Reference.Identity.Group, restore.Reference.APIVersion = "openbao.org", "openbao.org/v1alpha1"
			restore.AdapterContract = string(adapter.V1Alpha1Snapshot)
		case "unqualified":
			target.AdapterContract, restore.AdapterContract = string(
				adapter.KubeBaoV1Alpha1Unqualified,
			), string(
				adapter.KubeBaoV1Alpha1Unqualified,
			)
		case "spoofed":
			target.AdapterContract, restore.AdapterContract = string(adapter.V1Alpha1Snapshot), string(adapter.V1Alpha1Snapshot)
		case "other-version":
			target.Reference.APIVersion, restore.Reference.APIVersion = "kubebao.org/v2", "kubebao.org/v2"
		}
		deployment := fleet.Deployment{}
		attachRecovery(&deployment, target, []shared.Restore{restore})
		if (deployment.Recovery != nil) != (change == "valid") {
			t.Fatalf("native restore association changed its selected source boundary for %s", change)
		}
	}
	legacyPolicy := analysisPolicy(time.Now().UTC(), fleet.Policy{}, adapter.V1Alpha1Snapshot)
	if slices.Contains(legacyPolicy.ApprovedContracts, string(adapter.KubeBaoV1Alpha1Snapshot)) {
		t.Fatal("legacy enrollment approved another API group's capture")
	}
	at := *cluster.Reference.CollectedAt.Value
	input := diagnose.ClusterInput{
		Subject: cluster.Reference.Identity, AdapterContract: cluster.AdapterContract, Observation: &cluster,
		Collection: shared.Collection{
			Scope: shared.Scope{
				Origin: cluster.Reference.Identity.Origin, Group: "kubebao.org", Version: "v1alpha1",
				Resource: "openbaoclusters", Namespace: "team", Action: shared.ReadList,
			},
			Interval: shared.Interval{StartedAt: at, CompletedAt: at}, Outcome: shared.CollectionComplete, Items: 1,
		},
	}
	if _, err := diagnose.Analyze(diagnose.Capture{SchemaVersion: shared.SchemaVersion, Clusters: []diagnose.ClusterInput{input}},
		legacyPolicy); !errors.Is(err, diagnose.ErrUnsupportedVersion) {
		t.Fatal("legacy policy accepted native evidence")
	}
}

func TestReaderRejectsUnsupportedAPIGroupBeforeCredentialLoading(t *testing.T) {
	t.Parallel()
	_, err := New(
		config.Connection{APIGroup: "unsupported.example", Access: config.Access{Mode: config.AccessKubeconfig, Kubeconfig: "/absent"}},
		fleet.Policy{},
	)
	if err == nil || err.Error() != "unsupported enrolled operator API group" {
		t.Fatal("invalid API group reached credential loading or silently selected another contract")
	}
}
