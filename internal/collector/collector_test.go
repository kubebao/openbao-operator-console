package collector

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

func testCluster() map[string]any {
	return map[string]any{
		"apiVersion": "openbao.org/v1alpha1",
		"kind":       "OpenBaoCluster",
		"metadata": map[string]any{
			"name":              "bao",
			"namespace":         "team",
			"uid":               "original",
			"generation":        int64(4),
			"resourceVersion":   "17",
			"creationTimestamp": time.Now().Add(-time.Hour).UTC().Format(time.RFC3339),
		},
		"spec": map[string]any{"replicas": 3, "version": "2.7.0"},
		"status": map[string]any{
			"observedGeneration": 4,
			"readyReplicas":      3,
			"currentVersion":     "2.7.0",
			"conditions": []any{
				map[string]any{"type": "Available", "status": "True", "observedGeneration": 4},
				map[string]any{"type": "OpenBaoSealed", "status": "False", "observedGeneration": 4},
			},
		},
	}
}

func writeList(w http.ResponseWriter, resource string, items []any, next string) {
	kind := "OpenBaoClusterList"
	if resource == "openbaorestores" {
		kind = "OpenBaoRestoreList"
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).
		Encode(map[string]any{
			"apiVersion": "openbao.org/v1alpha1", "kind": kind,
			"metadata": map[string]any{"resourceVersion": "100", "continue": next}, "items": items,
		})
}

func testReader(t *testing.T, handler http.HandlerFunc) *Reader {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer reader" || r.Header.Get("User-Agent") == "" {
			t.Error("unexpected method or authentication")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		resource := strings.TrimPrefix(r.URL.Path, "/apis/openbao.org/v1alpha1/namespaces/team/")
		if resource != "openbaoclusters" && resource != "openbaorestores" {
			t.Errorf("read escaped declared scope: %s", r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("labelSelector") != "" {
			t.Error("unexpected collection limits or selector")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	reader, err := New(
		config.Connection{
			ID:         "one",
			Namespaces: []config.Namespace{{Name: "team", Environment: "production"}},
			Access: config.Access{
				Mode: config.AccessRemote,
				Credential: &config.Credential{
					Server: server.URL,
					Token:  "reader",
					CA:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})),
				},
			},
		},
		fleet.Policy{StartupGrace: time.Minute, MaxBackupAge: 24 * time.Hour},
	)
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestCollectorPaginationAndOptionalAPIs(t *testing.T) {
	reads := 0
	optional := http.StatusNotFound
	reader := testReader(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openbaorestores") {
			w.WriteHeader(optional)
			return
		}
		reads++
		cluster := testCluster()
		next := "next"
		if r.URL.Query().Get("continue") == "next" {
			cluster["metadata"].(map[string]any)["name"] = "second"
			cluster["metadata"].(map[string]any)["uid"] = "second-uid"
			next = ""
		}
		writeList(w, "openbaoclusters", []any{cluster}, next)
	})
	for _, code := range []int{404, 403} {
		optional = code
		batch := reader.Collect(context.Background(), time.Now())
		if batch.Connection.Collection != "current" || len(batch.Deployments) != 2 {
			t.Fatalf("inventory: %+v", batch)
		}
		want := fleet.CapabilityStateUnsupported
		if code == 403 {
			want = fleet.CapabilityStateDenied
		}
		if batch.Connection.Capabilities[1].State != want || batch.Connection.Capabilities[2].CheckedAt != nil {
			t.Fatal("optional API support was fabricated")
		}
		for _, d := range batch.Deployments {
			if !d.Evidence.AnalysisIncomplete || d.Recovery != nil {
				t.Fatal("optional incomplete evidence was concealed")
			}
		}
	}
	if reads != 8 {
		t.Fatalf("initial and verification pagination reads = %d", reads)
	}
}

func TestCollectorVerifiesTargetAndKeepsMissingEvidenceUnknown(t *testing.T) {
	for _, change := range []string{"unchanged", "generation", "deleted", "replacement", "malformed"} {
		t.Run(change, func(t *testing.T) {
			reads := 0
			reader := testReader(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "openbaorestores") {
					writeList(w, "openbaorestores", []any{}, "")
					return
				}
				reads++
				cluster := testCluster()
				items := []any{cluster}
				if reads == 2 {
					switch change {
					case "generation":
						cluster["metadata"].(map[string]any)["generation"] = 5
					case "deleted":
						items = []any{}
					case "replacement":
						cluster["metadata"].(map[string]any)["uid"] = "replacement"
					case "malformed":
						_, _ = w.Write([]byte(`{"kind":"List","items":[]}`))
						return
					}
				}
				writeList(w, "openbaoclusters", items, "")
			})
			batch := reader.Collect(context.Background(), time.Now())
			verifyChangedBatch(t, change, batch)
		})
	}
}

func verifyChangedBatch(t *testing.T, change string, batch store.Batch) {
	t.Helper()
	if change == "malformed" {
		if batch.Connection.Collection == "current" || len(batch.Deployments) != 0 {
			t.Fatal("incomplete verification replaced inventory")
		}
		return
	}
	if change == "deleted" || change == "replacement" {
		if len(batch.Deployments) != 0 {
			t.Fatal("old UID retained after verified deletion/replacement")
		}
		return
	}
	if len(batch.Deployments) != 1 {
		t.Fatalf("inventory: %+v", batch.Connection)
	}
	d := batch.Deployments[0]
	if change == "generation" {
		if !d.Evidence.AnalysisIncomplete || d.Health == fleet.HealthHealthy {
			t.Fatal("changed generation became current diagnosis")
		}
	} else if d.Health != fleet.HealthHealthy || d.Evidence.AnalysisIncomplete {
		t.Fatalf("complete diagnosis = %+v", d)
	}
}
