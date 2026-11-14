package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type backupCountScenario struct {
	name       string
	successAge time.Duration
	attemptAge time.Duration
	health     fleet.Health
	incomplete bool
}

func TestBackupFailureCountOmissionPreservesHealthAndAgeFindings(t *testing.T) {
	t.Parallel()
	for _, group := range []config.APIGroup{config.APIGroupOpenBao, config.APIGroupKubeBao} {
		for _, scenario := range []backupCountScenario{
			{"latest attempt succeeded", time.Hour, 2 * time.Hour, fleet.HealthHealthy, false},
			{"successful backup overdue", 48 * time.Hour, 49 * time.Hour, fleet.HealthAttention, false},
			{"later attempt unresolved", time.Hour, 30 * time.Minute, fleet.HealthUnknown, true},
		} {
			t.Run(string(group)+"/"+scenario.name, func(t *testing.T) {
				testBackupFailureCountOmission(t, group, scenario)
			})
		}
	}
}

func testBackupFailureCountOmission(t *testing.T, group config.APIGroup, scenario backupCountScenario) {
	t.Helper()
	t.Parallel()
	now := time.Now().UTC()
	cluster := groupCluster(group)
	cluster["spec"].(map[string]any)["backup"] = map[string]any{"schedule": "0 * * * *"}
	// Synthetic API evidence reproduces the operator's omitted zero counter.
	cluster["status"].(map[string]any)["backup"] = map[string]any{
		"lastBackupTime":  now.Add(-scenario.successAge).Format(time.RFC3339),
		"lastAttemptTime": now.Add(-scenario.attemptAge).Format(time.RFC3339),
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/apis/" + string(group) + "/v1alpha1/namespaces/team/"
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("reader escaped declared scope: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch resource := strings.TrimPrefix(r.URL.Path, prefix); resource {
		case "openbaoclusters":
			writeGroupList(w, group, resource, []any{cluster})
		case "openbaorestores":
			writeGroupList(w, group, resource, []any{})
		default:
			t.Errorf("reader requested an unexpected resource: %s", resource)
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	batch := groupReader(t, server, "backup-evidence", group).Collect(context.Background(), now)
	if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 1 {
		t.Fatal("backup evidence did not produce a current deployment")
	}
	deployment := batch.Deployments[0]
	if deployment.Health != scenario.health || deployment.Evidence.AnalysisIncomplete != scenario.incomplete {
		t.Fatalf("health = %s, incomplete = %t; want %s, %t", deployment.Health,
			deployment.Evidence.AnalysisIncomplete, scenario.health, scenario.incomplete)
	}
	if deployment.Backup == nil || (deployment.Backup.ConsecutiveFailures == nil) != scenario.incomplete {
		t.Fatal("projected backup counter lost its known or unknown state")
	}
	if deployment.Backup.ConsecutiveFailures != nil && *deployment.Backup.ConsecutiveFailures != 0 {
		t.Fatal("a successful latest attempt did not project zero failures")
	}
	if scenario.health == fleet.HealthAttention {
		if len(deployment.Issues) != 1 || deployment.Issues[0].Rule != fleet.RuleBackupOverdue {
			t.Fatal("an overdue successful backup lost its age finding")
		}
	} else if len(deployment.Issues) != 0 {
		t.Fatal("healthy or incomplete evidence produced a confirmed backup finding")
	}
}
