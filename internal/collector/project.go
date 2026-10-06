// Package collector contains the Kubernetes boundary and approved projections.
package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/dc-tec/openbao-operator-console/internal/config"
	"github.com/dc-tec/openbao-operator-console/internal/contract"
	"github.com/dc-tec/openbao-operator-console/internal/evidence"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,127}$`)
var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var decimalIndex = regexp.MustCompile(`^(0|[1-9][0-9]{0,19})$`)

func ResourceID(connection, namespace, name, uid string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{connection, namespace, name, uid}, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func text(object map[string]interface{}, fields ...string) string {
	value, found, err := unstructured.NestedString(object, fields...)
	if err != nil || !found || !identifier.MatchString(value) {
		return ""
	}
	return value
}

func optionalText(object map[string]interface{}, fields ...string) *string {
	value := text(object, fields...)
	if value == "" {
		return nil
	}
	return &value
}

func timestamp(object map[string]interface{}, fields ...string) *time.Time {
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

func count(object map[string]interface{}, fields ...string) *int32 {
	value, found, err := unstructured.NestedInt64(object, fields...)
	if err != nil || !found || value < 0 || value > math.MaxInt32 {
		return nil
	}
	result := int32(value)
	return &result
}

func boolean(object map[string]interface{}, fields ...string) bool {
	value, found, err := unstructured.NestedBool(object, fields...)
	return err == nil && found && value
}

func Project(cluster unstructured.Unstructured, connection config.Connection, namespace config.Namespace, policy evidence.Policy, now time.Time) contract.Deployment {
	object := cluster.Object
	result := contract.Deployment{
		Id:  ResourceID(connection.ID, cluster.GetNamespace(), cluster.GetName(), string(cluster.GetUID())),
		Uid: string(cluster.GetUID()), Name: cluster.GetName(), Namespace: cluster.GetNamespace(),
		ConnectionId: connection.ID, Team: namespace.Team, Location: namespace.Location,
		Environment: contract.DeploymentEnvironment(namespace.Environment), Generation: cluster.GetGeneration(),
		ResourceVersion: cluster.GetResourceVersion(), Version: text(object, "status", "currentVersion"),
		DesiredVersion: text(object, "spec", "version"), ReadyReplicas: count(object, "status", "readyReplicas"),
		DesiredReplicas: count(object, "spec", "replicas"), Role: "primary", Health: "unknown",
		Conditions: []contract.Condition{}, Issues: []contract.Issue{},
		Evidence: contract.Evidence{Collection: "current", CollectedAt: now, LastAttemptAt: now, ProducerFreshness: "unknown"},
	}
	if reason := text(object, "status", "upgrade", "failure", "reason"); reason != "" {
		result.UpgradeFailure = &contract.OperationFailure{Reason: reason, At: timestamp(object, "status", "upgrade", "failure", "at")}
	}
	if remote, found, err := unstructured.NestedMap(object, "spec", "remoteReplica"); err == nil && found && len(remote) > 0 {
		result.Role = "replica"
		if timestamp(object, "status", "recovery", "activatedAt") != nil {
			result.Role = "primary"
		}
	}
	conditions, _, _ := unstructured.NestedSlice(object, "status", "conditions")
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		status := contract.ConditionStatus(text(condition, "status"))
		if !status.Valid() {
			status = "Unknown"
		}
		generation, _, _ := unstructured.NestedInt64(condition, "observedGeneration")
		result.Conditions = append(result.Conditions, contract.Condition{
			Type: text(condition, "type"), Status: status, Reason: text(condition, "reason"),
			// Raw operator messages stay outside the approved projection; the reason is shown instead.
			Message:            "",
			ObservedGeneration: generation, LastTransitionAt: timestamp(condition, "lastTransitionTime"),
		})
	}
	if _, configured, err := unstructured.NestedMap(object, "spec", "backup"); err == nil && configured {
		backup := &contract.Backup{
			LastSuccessAt: timestamp(object, "status", "backup", "lastBackupTime"),
			NextAt:        timestamp(object, "status", "backup", "nextScheduledBackup"),
			Key:           optionalText(object, "status", "backup", "lastBackupName"),
		}
		if failures := count(object, "status", "backup", "consecutiveFailures"); failures != nil {
			backup.ConsecutiveFailures = *failures
		}
		if value, found, err := unstructured.NestedString(object, "status", "backup", "latestSnapshot", "digest"); err == nil && found && digestPattern.MatchString(value) {
			backup.Digest = &value
		}
		if size, found, err := unstructured.NestedInt64(object, "status", "backup", "lastBackupSize"); err == nil && found && size >= 0 {
			backup.SizeBytes = &size
		}
		if test, found, err := unstructured.NestedMap(object, "status", "backup", "restoreTest"); err == nil && found {
			projected := &contract.RestoreTest{
				Outcome: "notRun", LastSuccessAt: timestamp(test, "lastSuccessTime"),
				FinishedAt: timestamp(test, "last", "finishedAt"), Key: optionalText(test, "last", "key"),
				Reason: text(test, "last", "reason"),
			}
			switch text(test, "last", "outcome") {
			case "Passed":
				projected.Outcome = "passed"
			case "Failed":
				projected.Outcome = "failed"
			}
			if value, found, err := unstructured.NestedString(test, "last", "digest"); err == nil && found && digestPattern.MatchString(value) {
				projected.Digest = &value
			}
			if _, active, err := unstructured.NestedMap(test, "active"); err == nil && active {
				projected.Outcome = "running"
			}
			checks, _, _ := unstructured.NestedSlice(test, "conditions")
			for _, raw := range checks {
				check, ok := raw.(map[string]interface{})
				if !ok {
					continue
				}
				if text(check, "type") == "Passed" && text(check, "status") == "False" {
					projected.Reason = text(check, "reason")
					if projected.Reason != "Failed" {
						projected.Outcome = "blocked"
					}
				}
			}
			backup.RestoreTest = projected
		}
		result.Backup = backup
	}
	if observation, found, err := unstructured.NestedMap(object, "status", "remoteReplicaObservation"); err == nil && found {
		replica := &contract.Replica{ObservedAt: timestamp(observation, "observedAt"), HistoryConflict: boolean(observation, "historyConflict")}
		for path, target := range map[string]**string{"freshness": &replica.LocalIndex, "source": &replica.SourceIndex} {
			field := "appliedIndex"
			if path == "freshness" {
				field = "highestAppliedIndex"
			}
			value, found, err := unstructured.NestedString(observation, path, field)
			if _, parseErr := strconv.ParseUint(value, 10, 64); err == nil && found && decimalIndex.MatchString(value) && parseErr == nil {
				*target = &value
			}
		}
		result.Replica = replica
	}
	evidence.Evaluate(&result, cluster.GetCreationTimestamp().Time, boolean(object, "spec", "paused"), policy, now)
	return result
}

func ProjectRecovery(request unstructured.Unstructured, mechanism contract.RecoveryMechanism) *contract.Recovery {
	object := request.Object
	phase := contract.RecoveryPhase(strings.ToLower(text(object, "status", "phase")))
	if !phase.Valid() {
		phase = "unknown"
	}
	result := &contract.Recovery{
		RequestName: request.GetName(), Mechanism: mechanism, Phase: phase,
		Stage: text(object, "status", "execution", "stage"), StartedAt: timestamp(object, "status", "startTime"),
		CompletedAt: timestamp(object, "status", "completionTime"), Cleanup: "notApplicable",
		ApplicationConfirmedAt: timestamp(object, "status", "target", "appliedAt"),
	}
	if cleanup := contract.RecoveryCleanup(strings.ToLower(text(object, "status", "target", "cleanup"))); cleanup.Valid() {
		result.Cleanup = cleanup
	}
	if value, found, err := unstructured.NestedBool(object, "spec", "exclusionAcknowledged"); err == nil && found {
		result.ExclusionAcknowledged = &value
	}
	return result
}
