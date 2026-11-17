package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type conditionScenario struct {
	name, condition, status string
	generation              int
	readPool                bool
	health                  fleet.Health
	incomplete              bool
	rule                    fleet.Rule
	severity                fleet.Severity
}

func TestConditionFindingsAffectHealthWithoutBroaderReads(t *testing.T) {
	t.Parallel()
	for _, group := range []config.APIGroup{config.APIGroupOpenBao, config.APIGroupKubeBao} {
		for _, scenario := range []conditionScenario{
			{"unavailable", "Available", "False", 4, false, fleet.HealthAttention, false, fleet.RuleAvailability, fleet.SeverityHigh},
			{"degraded", "Degraded", "True", 4, false, fleet.HealthAttention, false, fleet.RuleOperatorCondition, fleet.SeverityHigh},
			{"TLS failure", "TLSReady", "False", 4, false, fleet.HealthAttention, false, fleet.RuleOperatorCondition, fleet.SeverityHigh},
			{
				"read pool unavailable", "ReadServingAvailable", "False", 4, true,
				fleet.HealthAttention, false, fleet.RuleOperatorCondition, fleet.SeverityHigh,
			},
			{"no read pool", "ReadServingAvailable", "False", 4, false, fleet.HealthHealthy, false, "", ""},
			{
				"advisory", "EtcdEncryptionWarning", "True", 4, false,
				fleet.HealthHealthy, false, fleet.RuleOperatorCondition, fleet.SeverityInfo,
			},
			{"normal", "TLSReady", "True", 4, false, fleet.HealthHealthy, false, "", ""},
			{"unknown", "TLSReady", "Unknown", 4, false, fleet.HealthUnknown, true, "", ""},
			{"stale", "TLSReady", "False", 3, false, fleet.HealthUnknown, true, "", ""},
			{"unacknowledged", "TLSReady", "False", 0, false, fleet.HealthUnknown, true, "", ""},
			{"stale availability", "Available", "False", 3, false, fleet.HealthUnknown, true, "", ""},
			{"unknown availability", "Available", "Unknown", 4, false, fleet.HealthUnknown, true, "", ""},
		} {
			t.Run(string(group)+"/"+scenario.name, func(t *testing.T) {
				t.Parallel()
				testConditionFinding(t, group, scenario)
			})
		}
	}
}

func testConditionFinding(t *testing.T, group config.APIGroup, scenario conditionScenario) {
	t.Helper()
	cluster := groupCluster(group)
	status := cluster["status"].(map[string]any)
	conditions := status["conditions"].([]any)
	if scenario.condition == "Available" {
		conditions = conditions[1:]
	}
	conditions = append(conditions, map[string]any{
		"type": scenario.condition, "status": scenario.status, "observedGeneration": scenario.generation,
		"reason": "Fixture", "message": "PRIVATE_CONDITION_CANARY",
	})
	status["conditions"] = conditions
	if scenario.readPool {
		cluster["spec"].(map[string]any)["readReplicas"] = map[string]any{"replicas": 2}
		status["readReplicas"] = map[string]any{"readyReplicas": 0}
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/apis/" + string(group) + "/v1alpha1/namespaces/team/"
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, prefix) {
			t.Errorf("collection escaped its declared read scope: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch resource := strings.TrimPrefix(r.URL.Path, prefix); resource {
		case "openbaoclusters":
			writeGroupList(w, group, resource, []any{cluster})
		case "openbaorestores":
			writeGroupList(w, group, resource, []any{})
		default:
			t.Errorf("unexpected resource: %s", resource)
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer server.Close()
	reader := groupReader(t, server, "condition-evidence", group)
	batch := reader.Collect(context.Background(), time.Now().UTC())
	if batch.Connection.Collection != fleet.ConnectionCollectionCurrent || len(batch.Deployments) != 1 {
		t.Fatalf("collection = %+v", batch.Connection)
	}
	deployment := batch.Deployments[0]
	if deployment.Health != scenario.health || deployment.Evidence.AnalysisIncomplete != scenario.incomplete {
		t.Fatalf("health = %s, incomplete = %t; want %s, %t", deployment.Health,
			deployment.Evidence.AnalysisIncomplete, scenario.health, scenario.incomplete)
	}
	if scenario.rule == "" {
		if len(deployment.Issues) != 0 {
			t.Fatalf("unexpected findings: %+v", deployment.Issues)
		}
	} else if len(deployment.Issues) != 1 || deployment.Issues[0].Rule != scenario.rule ||
		deployment.Issues[0].Severity != scenario.severity || deployment.Issues[0].ConditionType != scenario.condition {
		t.Fatalf("condition finding = %+v", deployment.Issues)
	}
	encoded, err := json.Marshal(batch)
	if err != nil || strings.Contains(string(encoded), "PRIVATE_CONDITION_CANARY") {
		t.Fatal("provider message reached the collected projection")
	}
}
