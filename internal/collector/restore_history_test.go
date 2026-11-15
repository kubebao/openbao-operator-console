package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
	"github.com/kubebao/openbao-operator-console/internal/store"
)

// The fixture drives the real collector and durable store together. It does not
// qualify an operator or execute a restore.
func TestCollectedRestoreEpisodesSurviveIncompleteReadsAndRestart(t *testing.T) {
	for _, group := range []config.APIGroup{config.APIGroupKubeBao, config.APIGroupOpenBao} {
		t.Run(string(group), func(t *testing.T) {
			testCollectedRestoreHistory(t, group)
		})
	}
}

func testCollectedRestoreHistory(t *testing.T, group config.APIGroup) {
	t.Helper()
	state := &restoreHistoryFixture{group: group, phase: "Running", uid: "restore-uid"}
	server := httptest.NewTLSServer(http.HandlerFunc(state.serve))
	defer server.Close()
	reader := groupReader(t, server, string(group), group)
	path := filepath.Join(t.TempDir(), "console.db")
	repository := openRestoreHistory(t, path)
	defer func() { repository.Close() }()
	collect := func() fleet.Deployment { return collectRestoreHistory(t, reader, repository, state.mode) }

	progress := collect()
	initial := restoreEpisode(t, progress)
	if initial.Severity != fleet.SeverityInfo || progress.Recovery == nil ||
		progress.Recovery.Phase != fleet.RecoveryPhaseRunning || progress.Evidence.AnalysisIncomplete {
		t.Fatal("a UID-confirmed in-progress restore lost its presentation or complete coverage")
	}

	state.phase, state.held = "Failed", true
	failed := collect()
	failure := restoreEpisode(t, failed)
	if failure.ID != initial.ID || !failure.FirstSeenAt.Equal(initial.FirstSeenAt) || failure.Severity != fleet.SeverityHigh ||
		failed.Recovery == nil || failed.Recovery.Phase != fleet.RecoveryPhaseFailed {
		t.Fatal("a changed restore outcome replaced its finding identity or first-seen time")
	}

	for _, mode := range []string{"denied", "partial"} {
		state.mode = mode
		incomplete := collect()
		episode := restoreEpisode(t, incomplete)
		if !incomplete.Evidence.AnalysisIncomplete || episode.ID != initial.ID ||
			!episode.FirstSeenAt.Equal(initial.FirstSeenAt) || incomplete.Recovery == nil ||
			incomplete.Recovery.Phase != fleet.RecoveryPhaseFailed {
			t.Fatalf("%s restore reads concealed or resolved the last confirmed episode", mode)
		}
		if restoreActivityCount(t, repository, group, fleet.ActivityFindingResolved) != 0 {
			t.Fatal("incomplete restore evidence recorded a finding resolution")
		}
	}

	beforeRestart, err := repository.History(context.Background(), failed.ID, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
	if err != nil {
		t.Fatal(err)
	}
	repository.Close()
	repository = openRestoreHistory(t, path)
	restarted := storedRestoreDeployment(t, repository)
	if episode := restoreEpisode(t, restarted); episode.ID != initial.ID || !episode.FirstSeenAt.Equal(initial.FirstSeenAt) {
		t.Fatal("restart lost the unresolved restore episode")
	}
	afterRestart, err := repository.History(context.Background(), failed.ID, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
	if err != nil || !reflect.DeepEqual(beforeRestart, afterRestart) || !reflect.DeepEqual(failed.Recovery, restarted.Recovery) {
		t.Fatal("opening persisted fleet state changed restore history or the retained recovery projection")
	}
	state.mode = ""
	confirmed := collect()
	if episode := restoreEpisode(t, confirmed); confirmed.Evidence.AnalysisIncomplete || episode.ID != initial.ID ||
		!episode.FirstSeenAt.Equal(initial.FirstSeenAt) {
		t.Fatal("restored read access replaced the existing restore episode")
	}

	state.uid, state.phase, state.held = "replacement-restore-uid", "Running", false
	replacement := collect()
	newEpisode := restoreEpisode(t, replacement)
	if newEpisode.ID == initial.ID || !newEpisode.FirstSeenAt.After(initial.FirstSeenAt) ||
		replacement.ID != progress.ID || replacement.Recovery == nil || replacement.Recovery.RequestName != "restore" {
		t.Fatal("a new request UID with the same name inherited its predecessor's finding identity")
	}
	if restoreActivityCount(t, repository, group, fleet.ActivityFindingResolved) != 1 {
		t.Fatal("the replaced request did not resolve its own episode")
	}

	state.phase = "Completed"
	completed := collect()
	if len(completed.Issues) != 0 || completed.Evidence.AnalysisIncomplete || completed.Recovery == nil ||
		completed.Recovery.Phase != fleet.RecoveryPhaseCompleted || completed.Recovery.Cleanup != fleet.RecoveryCleanupUnknown {
		t.Fatal("a completed request lost its reported phase or fabricated cleanup proof")
	}
	if restoreActivityCount(t, repository, group, fleet.ActivityFindingResolved) != 2 {
		t.Fatal("the completed request did not resolve its own episode")
	}
	history, err := repository.History(context.Background(), completed.ID, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
	if err != nil || len(history.Observations) < 4 || history.Observations[0].Deployment.Recovery == nil ||
		history.Observations[0].Deployment.Recovery.Phase != fleet.RecoveryPhaseCompleted {
		t.Fatal("restore progress and resolution did not retain durable history")
	}
	encoded, err := json.Marshal(history)
	if err != nil || strings.Contains(string(encoded), "PRIVATE_PROVIDER_CANARY") {
		t.Fatal("provider text entered durable restore history")
	}
}

func collectRestoreHistory(t *testing.T, reader *Reader, repository *store.SQLite, mode string) fleet.Deployment {
	t.Helper()
	ctx := context.Background()
	batch := reader.Collect(ctx, time.Now().UTC())
	if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 1 {
		t.Fatal("restore observation lost the current cluster inventory")
	}
	if mode != "" && !slices.Contains(batch.Unresolved[batch.Deployments[0].ID], fleet.RuleRecoveryInvestigate) {
		t.Fatal("an incomplete restore read did not retain unresolved restore coverage")
	}
	if err := repository.Apply(ctx, batch); err != nil {
		t.Fatal(err)
	}
	return storedRestoreDeployment(t, repository)
}

type restoreHistoryFixture struct {
	group      config.APIGroup
	phase      string
	uid        string
	mode       string
	held       bool
	disposable bool
	retention  int
	deleted    bool
}

func (f *restoreHistoryFixture) serve(w http.ResponseWriter, r *http.Request) {
	prefix := "/apis/" + string(f.group) + "/v1alpha1/namespaces/team/"
	if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer reader" ||
		r.URL.Query().Get("limit") != "200" || r.URL.Query().Get("labelSelector") != "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	switch strings.TrimPrefix(r.URL.Path, prefix) {
	case "openbaoclusters":
		if f.deleted {
			writeGroupList(w, f.group, "openbaoclusters", []any{})
			return
		}
		cluster := groupCluster(f.group)
		annotations := map[string]any{"openbao.org/restore-origin": f.uid}
		if f.held {
			annotations["openbao.org/restore-hold"] = f.uid
		}
		cluster["metadata"].(map[string]any)["annotations"] = annotations
		writeGroupList(w, f.group, "openbaoclusters", []any{cluster})
	case "openbaorestores":
		if f.mode == "denied" || r.URL.Query().Get("continue") != "" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("PRIVATE_PROVIDER_CANARY"))
			return
		}
		request := groupRestore(f.group)
		request["metadata"].(map[string]any)["uid"] = f.uid
		request["status"].(map[string]any)["phase"] = f.phase
		if f.disposable {
			restoreSpec := request["spec"].(map[string]any)
			restoreSpec["targetLifecycle"], restoreSpec["cleanupAfterSeconds"] = "Disposable", f.retention
			status := request["status"].(map[string]any)
			if f.phase == "Unknown" || f.phase == "Completed" {
				status["target"] = map[string]any{
					"uid": "original", "appliedAt": time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339), "cleanup": "Pending",
				}
				status["execution"] = map[string]any{"targetUID": "original", "stage": f.phase, "terminalResult": "Succeeded"}
				if f.phase == "Completed" {
					status["target"].(map[string]any)["cleanup"] = "Complete"
					status["execution"].(map[string]any)["stage"] = "FollowThroughComplete"
				}
			}
		}
		next := ""
		if f.mode == "partial" {
			next = "next"
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"apiVersion": string(f.group) + "/v1alpha1", "kind": "OpenBaoRestoreList",
			"metadata": map[string]any{"resourceVersion": "100", "continue": next}, "items": []any{request},
		})
	default:
		w.WriteHeader(http.StatusForbidden)
	}
}

func TestDisposableRetentionPreservesUnknownPresentation(t *testing.T) {
	for _, group := range []config.APIGroup{config.APIGroupKubeBao, config.APIGroupOpenBao} {
		t.Run(string(group), func(t *testing.T) {
			for _, test := range []struct {
				name         string
				disposable   bool
				wantSeverity fleet.Severity
			}{
				{name: "retention-pending", disposable: true, wantSeverity: fleet.SeverityInfo},
				{name: "unknown-without-retention-proof", wantSeverity: fleet.SeverityHigh},
			} {
				t.Run(test.name, func(t *testing.T) {
					state := &restoreHistoryFixture{
						group: group, phase: "Unknown", uid: "restore-uid", held: true, disposable: test.disposable, retention: 3600,
					}
					server := httptest.NewTLSServer(http.HandlerFunc(state.serve))
					defer server.Close()
					batch := groupReader(t, server, string(group), group).Collect(context.Background(), time.Now().UTC())
					if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 1 {
						t.Fatal("retention observation lost the current target")
					}
					deployment := batch.Deployments[0]
					issue := restoreEpisode(t, deployment)
					if issue.Severity != test.wantSeverity || deployment.Evidence.AnalysisIncomplete || deployment.Recovery == nil ||
						deployment.Recovery.Phase != fleet.RecoveryPhaseUnknown {
						t.Fatal("retention interpretation changed the reported phase or lost its console severity")
					}
					if test.disposable && (deployment.Recovery.ApplicationConfirmedAt == nil ||
						deployment.Recovery.Cleanup != fleet.RecoveryCleanupPending) {
						t.Fatal("reported application and pending cleanup were lost")
					}
				})
			}
		})
	}
}

func TestCompletedReceiptDoesNotRecreateDeletedDeployment(t *testing.T) {
	for _, group := range []config.APIGroup{config.APIGroupKubeBao, config.APIGroupOpenBao} {
		t.Run(string(group), func(t *testing.T) { testCompletedReceiptDeletion(t, group) })
	}
}

func testCompletedReceiptDeletion(t *testing.T, group config.APIGroup) {
	t.Helper()
	state := &restoreHistoryFixture{group: group, phase: "Running", uid: "restore-uid", disposable: true}
	server := httptest.NewTLSServer(http.HandlerFunc(state.serve))
	defer server.Close()
	reader := groupReader(t, server, string(group), group)
	repository := openRestoreHistory(t, filepath.Join(t.TempDir(), "console.db"))
	defer repository.Close()
	initial := collectRestoreHistory(t, reader, repository, "")
	state.phase, state.deleted = "Completed", true
	for range 2 {
		batch := reader.Collect(context.Background(), time.Now().UTC())
		if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 0 {
			t.Fatal("a retained receipt recreated its deleted target")
		}
		if err := repository.Apply(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
		snapshot, err := repository.Snapshot(context.Background())
		if err != nil || len(snapshot.Deployments) != 0 {
			t.Fatal("a retained receipt recreated a current deployment after persistence")
		}
	}
	history, err := repository.History(context.Background(), initial.ID, fleet.HistoryQuery{Page: fleet.Page{Limit: 100}})
	if err != nil || len(history.Observations) != 1 {
		t.Fatal("a retained receipt replaced or extended the deleted target's history")
	}
	retained := history.Observations[0].Deployment
	if retained.ID != initial.ID || retained.UID != initial.UID || retained.Recovery == nil ||
		retained.Recovery.Phase != fleet.RecoveryPhaseRunning {
		t.Fatal("the retained receipt rewrote historical recovery state or identity")
	}
}

func openRestoreHistory(t *testing.T, path string) *store.SQLite {
	t.Helper()
	repository, err := store.OpenSQLite(context.Background(), path, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func storedRestoreDeployment(t *testing.T, repository *store.SQLite) fleet.Deployment {
	t.Helper()
	snapshot, err := repository.Snapshot(context.Background())
	if err != nil || len(snapshot.Deployments) != 1 {
		t.Fatal("restore deployment was not durably retained")
	}
	return snapshot.Deployments[0]
}

func restoreEpisode(t *testing.T, deployment fleet.Deployment) fleet.Issue {
	t.Helper()
	if len(deployment.Issues) != 1 || deployment.Issues[0].Rule != fleet.RuleRecoveryInvestigate {
		t.Fatal("restore finding was lost or merged with another request")
	}
	return deployment.Issues[0]
}

func restoreActivityCount(t *testing.T, repository *store.SQLite, group config.APIGroup, kind fleet.ActivityKind) int {
	t.Helper()
	activity, err := repository.Activity(context.Background(), []config.Scope{{ConnectionID: string(group), Namespaces: []string{"team"}}},
		fleet.Page{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range activity.Events {
		if event.Kind == kind && event.Finding != nil && event.Finding.Rule == fleet.RuleRecoveryInvestigate {
			count++
		}
	}
	return count
}
