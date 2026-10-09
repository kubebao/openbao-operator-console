// Package collector contains the Kubernetes boundary and approved projections.
package collector

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

var (
	identifier    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	decimalIndex  = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})$`)
)

func text(object map[string]any, fields ...string) string {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || !identifier.MatchString(value) {
		return ""
	}
	return value
}

func optionalText(object map[string]any, fields ...string) *string {
	value := text(object, fields...)
	if value == "" {
		return nil
	}
	return &value
}

func digest(object map[string]any, fields ...string) *string {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || !digestPattern.MatchString(value) {
		return nil
	}
	return &value
}

func timestamp(object map[string]any, fields ...string) *time.Time {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func count(object map[string]any, fields ...string) *int32 {
	value, found, err := unstructured.NestedInt64(object, fields...)
	if err != nil || !found || value < 0 || value > math.MaxInt32 {
		return nil
	}
	result := int32(value)
	return &result
}

func boolean(object map[string]any, fields ...string) bool {
	value, found, err := unstructured.NestedBool(object, fields...)
	return err == nil && found && value
}

// Project maps an OpenBaoCluster to approved fields. Evidence rules run separately,
// after related recovery requests are attached.
func Project(cluster unstructured.Unstructured, connection config.Connection, namespace config.Namespace, now time.Time) fleet.Deployment {
	object := cluster.Object
	result := fleet.Deployment{
		ID:              fleet.DeploymentID(connection.ID, cluster.GetNamespace(), cluster.GetName(), string(cluster.GetUID())),
		UID:             string(cluster.GetUID()),
		Name:            cluster.GetName(),
		Namespace:       cluster.GetNamespace(),
		ConnectionID:    connection.ID,
		Team:            namespace.Team,
		Environment:     namespace.Environment,
		Location:        namespace.Location,
		Generation:      cluster.GetGeneration(),
		ResourceVersion: cluster.GetResourceVersion(),
		Version:         text(object, "status", "currentVersion"),
		DesiredVersion:  text(object, "spec", "version"),
		ReadyReplicas:   count(object, "status", "readyReplicas"),
		DesiredReplicas: count(object, "spec", "replicas"),
		Role:            projectRole(object),
		Health:          fleet.HealthUnknown,
		Conditions:      projectConditions(object),
		Issues:          []fleet.Issue{},
		Evidence: fleet.Evidence{
			Collection:        fleet.EvidenceCollectionCurrent,
			CollectedAt:       now,
			LastAttemptAt:     now,
			ProducerFreshness: fleet.FreshnessUnknown,
		},
		Backup:  projectBackup(object),
		Replica: projectReplica(object),
	}
	if reason := text(object, "status", "upgrade", "failure", "reason"); reason != "" {
		result.UpgradeFailure = &fleet.OperationFailure{Reason: reason, At: timestamp(object, "status", "upgrade", "failure", "at")}
	}
	return result
}

// A remote replica becomes a primary once the operator records its activation.
func projectRole(object map[string]any) fleet.Role {
	remote, found, err := unstructured.NestedMap(object, "spec", "remoteReplica")
	if err != nil || !found || len(remote) == 0 || timestamp(object, "status", "recovery", "activatedAt") != nil {
		return fleet.RolePrimary
	}
	return fleet.RoleReplica
}

func projectConditions(object map[string]any) []fleet.Condition {
	result := []fleet.Condition{}
	conditions, _, _ := unstructured.NestedSlice(object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		status := fleet.ConditionStatus(text(condition, "status"))
		if !status.Valid() {
			status = fleet.ConditionStatusUnknown
		}
		generation, _, _ := unstructured.NestedInt64(condition, "observedGeneration")
		result = append(result, fleet.Condition{
			Type:               text(condition, "type"),
			Status:             status,
			Reason:             text(condition, "reason"),
			ObservedGeneration: generation,
			LastTransitionAt:   timestamp(condition, "lastTransitionTime"),
		})
	}
	return result
}

func projectBackup(object map[string]any) *fleet.Backup {
	if _, configured, err := unstructured.NestedMap(object, "spec", "backup"); err != nil || !configured {
		return nil
	}
	backup := &fleet.Backup{
		LastSuccessAt: timestamp(object, "status", "backup", "lastBackupTime"),
		NextAt:        timestamp(object, "status", "backup", "nextScheduledBackup"),
		Key:           optionalText(object, "status", "backup", "lastBackupName"),
		Digest:        digest(object, "status", "backup", "latestSnapshot", "digest"),
	}
	if failures := count(object, "status", "backup", "consecutiveFailures"); failures != nil {
		backup.ConsecutiveFailures = *failures
	}
	if size, found, err := unstructured.NestedInt64(object, "status", "backup", "lastBackupSize"); err == nil && found && size >= 0 {
		backup.SizeBytes = &size
	}
	if test, found, err := unstructured.NestedMap(object, "status", "backup", "restoreTest"); err == nil && found {
		backup.RestoreTest = projectRestoreTest(test)
	}
	return backup
}

func projectRestoreTest(test map[string]any) *fleet.RestoreTest {
	result := &fleet.RestoreTest{
		Outcome:       fleet.RestoreOutcomeNotRun,
		LastSuccessAt: timestamp(test, "lastSuccessTime"),
		FinishedAt:    timestamp(test, "last", "finishedAt"),
		Key:           optionalText(test, "last", "key"),
		Reason:        text(test, "last", "reason"),
		Digest:        digest(test, "last", "digest"),
	}
	switch text(test, "last", "outcome") {
	case "Passed":
		result.Outcome = fleet.RestoreOutcomePassed
	case "Failed":
		result.Outcome = fleet.RestoreOutcomeFailed
	}
	if _, active, err := unstructured.NestedMap(test, "active"); err == nil && active {
		result.Outcome = fleet.RestoreOutcomeRunning
	}
	// A failed Passed condition with any reason other than Failed blocks the test.
	checks, _, _ := unstructured.NestedSlice(test, "conditions")
	for _, raw := range checks {
		check, ok := raw.(map[string]any)
		if !ok || text(check, "type") != "Passed" || text(check, "status") != "False" {
			continue
		}
		result.Reason = text(check, "reason")
		if result.Reason != "Failed" {
			result.Outcome = fleet.RestoreOutcomeBlocked
		}
	}
	return result
}

func projectReplica(object map[string]any) *fleet.Replica {
	observation, found, err := unstructured.NestedMap(object, "status", "remoteReplicaObservation")
	if err != nil || !found {
		return nil
	}
	return &fleet.Replica{
		ObservedAt:      timestamp(observation, "observedAt"),
		HistoryConflict: boolean(observation, "historyConflict"),
		LocalIndex:      appliedIndex(observation, "freshness", "highestAppliedIndex"),
		SourceIndex:     appliedIndex(observation, "source", "appliedIndex"),
	}
}

// Raft indexes exceed the JSON safe integer range, so they stay exact decimal strings.
func appliedIndex(object map[string]any, fields ...string) *string {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || !decimalIndex.MatchString(value) {
		return nil
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return nil
	}
	return &value
}

func ProjectRecovery(request unstructured.Unstructured, mechanism fleet.RecoveryMechanism) *fleet.Recovery {
	object := request.Object
	phase := fleet.RecoveryPhase(strings.ToLower(text(object, "status", "phase")))
	if !phase.Valid() {
		phase = fleet.RecoveryPhaseUnknown
	}
	result := &fleet.Recovery{
		RequestName:            request.GetName(),
		Mechanism:              mechanism,
		Phase:                  phase,
		Stage:                  text(object, "status", "execution", "stage"),
		StartedAt:              timestamp(object, "status", "startTime"),
		CompletedAt:            timestamp(object, "status", "completionTime"),
		Cleanup:                fleet.RecoveryCleanupNotApplicable,
		ApplicationConfirmedAt: timestamp(object, "status", "target", "appliedAt"),
	}
	if cleanup := fleet.RecoveryCleanup(strings.ToLower(text(object, "status", "target", "cleanup"))); cleanup.Valid() {
		result.Cleanup = cleanup
	}
	if value, found, err := unstructured.NestedBool(object, "spec", "exclusionAcknowledged"); err == nil && found {
		result.ExclusionAcknowledged = &value
	}
	return result
}
