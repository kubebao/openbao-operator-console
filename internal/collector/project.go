package collector

import (
	"math"
	"strings"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func integerValue(value shared.Integer) int64 {
	if value.Value == nil {
		return 0
	}
	return *value.Value
}

func textValue(value shared.Text) string {
	if value.Value == nil {
		return ""
	}
	return *value.Value
}

func countValue(value shared.Integer) *int32 {
	if value.Value == nil || *value.Value < 0 || *value.Value > math.MaxInt32 {
		return nil
	}
	n := int32(*value.Value)
	return &n
}

// projectCluster maps approved shared facts into the existing persisted model.
// It does not evaluate diagnostics or infer missing producer values.
func projectCluster(source shared.Cluster, connection config.Connection, namespace config.Namespace) fleet.Deployment {
	id := source.Reference.Identity
	result := fleet.Deployment{
		ID: fleet.DeploymentID(connection.ID, id.Namespace, id.Name, id.UID), UID: id.UID, Name: id.Name, Namespace: id.Namespace,
		ConnectionID: connection.ID, Team: namespace.Team, Environment: namespace.Environment, Location: namespace.Location,
		Generation: integerValue(source.Generation), ResourceVersion: textValue(source.Reference.ResourceVersion),
		ObservedGeneration: source.ObservedGeneration.Value,
		Version:            textValue(source.ObservedVersion), DesiredVersion: textValue(source.DesiredVersion),
		ReadyReplicas: countValue(source.ReadyReplicas), DesiredReplicas: countValue(source.DesiredReplicas),
		Role: fleet.RolePrimary, Health: fleet.HealthUnknown, Conditions: []fleet.Condition{}, Issues: []fleet.Issue{},
		Evidence: fleet.Evidence{
			Collection: fleet.EvidenceCollectionCurrent, CollectedAt: *source.Reference.CollectedAt.Value,
			LastAttemptAt: *source.Reference.CollectedAt.Value, ProducerFreshness: fleet.FreshnessUnknown,
		},
	}
	for _, condition := range source.Conditions.Values {
		result.Conditions = append(result.Conditions, fleet.Condition{
			Type:   condition.Type,
			Status: fleet.ConditionStatus(condition.Status),
			Reason: textValue(
				condition.Reason,
			),
			ObservedGeneration: integerValue(condition.ObservedGeneration),
			LastTransitionAt:   condition.LastTransitionAt.Value,
		})
	}
	if source.Upgrade.Failure.State == shared.ObjectReported && source.Upgrade.Failure.Reason.State == shared.ValueReported {
		result.UpgradeFailure = &fleet.OperationFailure{
			Reason: textValue(source.Upgrade.Failure.Reason),
			At:     source.Upgrade.Failure.At.Value,
		}
	}
	if source.Backup.Configuration == shared.ObjectReported {
		b := source.Backup
		result.Backup = &fleet.Backup{
			LastSuccessAt: b.LastSuccessAt.Value, NextAt: b.NextScheduledAt.Value, Key: b.SnapshotKey.Value,
			ConsecutiveFailures: countValue(b.ConsecutiveFailures), Digest: b.SnapshotDigest.Value, SizeBytes: b.SizeBytes.Value,
		}
		if b.RestoreTest.State == shared.ObjectReported {
			test := b.RestoreTest
			outcome := fleet.RestoreOutcomeUnknown
			switch {
			case test.Active == shared.ObjectReported:
				outcome = fleet.RestoreOutcomeRunning
			case test.LastOutcome.State == shared.ValueReported:
				outcome = fleet.RestoreOutcome(strings.ToLower(*test.LastOutcome.Value))
			case test.Active == shared.ObjectAbsent && test.Last == shared.ObjectAbsent:
				outcome = fleet.RestoreOutcomeNotRun
			}
			result.Backup.RestoreTest = &fleet.RestoreTest{
				Outcome: outcome, LastSuccessAt: test.LastSuccessAt.Value, FinishedAt: test.FinishedAt.Value,
				Key: test.SnapshotKey.Value, Reason: textValue(test.LastReason), Digest: test.SnapshotDigest.Value,
			}
		}
	}
	return result
}

// applyRestoreTestPresentation uses the rule's confirmed condition reference to
// distinguish a blocked test without implementing its interpretation a second time.
func applyRestoreTestPresentation(deployment *fleet.Deployment, source shared.Cluster, report diagnose.Report) {
	if deployment.Backup == nil || deployment.Backup.RestoreTest == nil {
		return
	}
	for _, finding := range report.Findings {
		if finding.Rule != diagnose.RuleRestoreTest {
			continue
		}
		for _, ref := range finding.References {
			if ref.Field != "status.backup.restoreTest.conditions[type=Passed]" {
				continue
			}
			deployment.Backup.RestoreTest.Outcome = fleet.RestoreOutcomeBlocked
			for _, condition := range source.Backup.RestoreTest.Conditions.Values {
				if condition.Type == "Passed" {
					deployment.Backup.RestoreTest.Reason = textValue(condition.Reason)
				}
			}
		}
	}
}

func projectRecovery(request shared.Restore) *fleet.Recovery {
	phase := fleet.RecoveryPhaseUnknown
	if request.Phase.State == shared.ValueReported {
		phase = fleet.RecoveryPhase(strings.ToLower(*request.Phase.Value))
	}
	cleanup := fleet.RecoveryCleanupUnknown
	if request.Target.Cleanup.State == shared.ValueReported {
		cleanup = fleet.RecoveryCleanup(strings.ToLower(*request.Target.Cleanup.Value))
	}
	return &fleet.Recovery{
		RequestName: request.Reference.Identity.Name, Mechanism: fleet.RecoveryMechanismSnapshot, Phase: phase,
		Stage: textValue(request.Execution.Stage), StartedAt: request.StartedAt.Value, CompletedAt: request.CompletedAt.Value,
		Cleanup: cleanup, ApplicationConfirmedAt: request.Target.AppliedAt.Value,
	}
}

func attachRecovery(deployment *fleet.Deployment, cluster shared.Cluster, requests []shared.Restore) {
	latest, found, err := diagnose.LatestRestore(cluster, requests)
	if err != nil || !found {
		return
	}
	deployment.Recovery = projectRecovery(latest)
	if cluster.RestoreOrigin.State == shared.ValueReported && *cluster.RestoreOrigin.Value == latest.Reference.Identity.UID {
		deployment.Role = fleet.RoleRecoveryTarget
	}
}

func contractForGroup(group config.APIGroup) adapter.Contract {
	switch group {
	case config.APIGroupOpenBao:
		return adapter.OpenBaoV1Alpha1
	case config.APIGroupKubeBao:
		return adapter.KubeBaoV1Alpha1
	default:
		return ""
	}
}

func analysisPolicy(at time.Time, policy fleet.Policy) diagnose.Policy {
	result := diagnose.DefaultPolicy(at)
	if policy.MaxBackupAge > 0 {
		result.MaxBackupAge = policy.MaxBackupAge
	}
	if policy.StartupGrace > 0 {
		result.StartupGrace = policy.StartupGrace
	} else {
		result.StartupGrace = 0
	}
	result.MaxSubjects = 10001
	return result
}
