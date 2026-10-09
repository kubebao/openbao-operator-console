// Package evidence evaluates typed observations without executing operations.
package evidence

import (
	"fmt"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// Evaluate uses current evidence only. The store preserves unresolved findings
// when collection fails instead of evaluating a missing observation as recovery.
func Evaluate(deployment *fleet.Deployment, createdAt time.Time, paused bool, policy fleet.Policy, now time.Time) {
	deployment.Issues = []fleet.Issue{}
	add := func(rule fleet.Rule, severity fleet.Severity, category fleet.Category, title, detail string) {
		deployment.Issues = append(deployment.Issues, fleet.Issue{
			ID: deployment.ID + ":" + string(rule), DeploymentID: deployment.ID, Rule: rule,
			Severity: severity, Category: category, Title: title, Detail: detail,
			FirstSeenAt: now, LastCheckedAt: now,
		})
	}
	deployment.Health = fleet.HealthUnknown
	if deployment.ReadyReplicas != nil && deployment.DesiredReplicas != nil && *deployment.DesiredReplicas > 0 {
		if *deployment.ReadyReplicas >= *deployment.DesiredReplicas {
			deployment.Health = fleet.HealthHealthy
		}
		if !paused && now.Sub(createdAt) >= policy.StartupGrace && *deployment.ReadyReplicas < *deployment.DesiredReplicas {
			severity := fleet.SeverityHigh
			if *deployment.ReadyReplicas == 0 {
				severity = fleet.SeverityCritical
			}
			add(
				fleet.RuleReplicaReadiness,
				severity,
				fleet.CategoryAvailability,
				"Replicas need attention",
				fmt.Sprintf(
					"%d of %d replicas are ready. This is workload readiness evidence, not a client transaction check.",
					*deployment.ReadyReplicas,
					*deployment.DesiredReplicas,
				),
			)
		}
	}
	for _, condition := range deployment.Conditions {
		if condition.ObservedGeneration < deployment.Generation {
			continue
		}
		if !paused && condition.Type == "OpenBaoSealed" && condition.Status == fleet.ConditionStatusTrue {
			add(
				fleet.RuleSealed,
				fleet.SeverityCritical,
				fleet.CategoryAvailability,
				"OpenBao is sealed",
				"The operator reports a sealed deployment. Investigate unseal access and current workload evidence.",
			)
		}
	}
	if failure := deployment.UpgradeFailure; failure != nil {
		add(
			fleet.RuleUpgradeFailed,
			fleet.SeverityHigh,
			fleet.CategoryOperations,
			"Upgrade requires investigation",
			"The operator records a failed upgrade with reason "+failure.Reason+". Inspect its execution evidence.",
		)
	}
	if backup := deployment.Backup; backup != nil {
		switch {
		case backup.ConsecutiveFailures > 0:
			add(
				fleet.RuleBackupFailed,
				fleet.SeverityHigh,
				fleet.CategoryBackup,
				"Backup failed",
				fmt.Sprintf(
					"%d consecutive backup attempts failed. Last success and latest failure are separate evidence.",
					backup.ConsecutiveFailures,
				),
			)
		case backup.LastSuccessAt == nil:
			add(
				fleet.RuleBackupNeverSucceeded,
				fleet.SeverityHigh,
				fleet.CategoryBackup,
				"No successful backup observed",
				"Backups are configured, but no successful completion is recorded.",
			)
		case now.Sub(*backup.LastSuccessAt) > policy.MaxBackupAge:
			add(
				fleet.RuleBackupOverdue,
				fleet.SeverityHigh,
				fleet.CategoryBackup,
				"Backup is overdue",
				fmt.Sprintf(
					"Last successful completion exceeds the configured %.0f-hour threshold. Completion age is not measured RPO.",
					policy.MaxBackupAge.Hours(),
				),
			)
		}
		if test := backup.RestoreTest; test != nil &&
			(test.Outcome == fleet.RestoreOutcomeFailed || test.Outcome == fleet.RestoreOutcomeBlocked) {
			add(
				fleet.RuleRestoreTest,
				fleet.SeverityHigh,
				fleet.CategoryBackup,
				"Restore test needs attention",
				"The latest restore test failed or is blocked. Its result covers the pinned snapshot and disposable target only.",
			)
		}
	}
	if replica := deployment.Replica; replica != nil && replica.HistoryConflict {
		add(
			fleet.RuleReplicaHistoryConflict,
			fleet.SeverityCritical,
			fleet.CategoryOperations,
			"Replica history conflict",
			"The remote replica observation records conflicting histories. An applied index does not authorize survivor selection.",
		)
	}
	if recovery := deployment.Recovery; recovery != nil &&
		(recovery.Phase == fleet.RecoveryPhaseUnknown || recovery.Phase == fleet.RecoveryPhaseFailed ||
			recovery.Cleanup == fleet.RecoveryCleanupFailed) {
		add(
			fleet.RuleRecoveryInvestigate,
			fleet.SeverityHigh,
			fleet.CategoryOperations,
			"Recovery requires investigation",
			"Execution or cleanup requires investigation. A recorded submission claim never authorizes replay.",
		)
	}
	for _, issue := range deployment.Issues {
		if issue.Severity == fleet.SeverityCritical {
			deployment.Health = fleet.HealthCritical
			break
		}
		if issue.Severity == fleet.SeverityHigh {
			deployment.Health = fleet.HealthAttention
		}
	}
}

func CollectionFailure(deployment *fleet.Deployment, state fleet.EvidenceCollection, now time.Time) {
	deployment.Evidence.Collection = state
	deployment.Evidence.LastAttemptAt = now
	for i := range deployment.Issues {
		if deployment.Issues[i].Rule == fleet.RuleCollection {
			deployment.Issues[i].LastCheckedAt = now
			return
		}
	}
	deployment.Issues = append(deployment.Issues, fleet.Issue{
		ID: deployment.ID + ":" + string(fleet.RuleCollection), DeploymentID: deployment.ID, Rule: fleet.RuleCollection,
		Severity: fleet.SeverityHigh, Category: fleet.CategoryVisibility, Title: "Collection is incomplete",
		Detail:      "The connector cannot refresh this deployment. Workload state and unresolved findings are last known.",
		FirstSeenAt: now, LastCheckedAt: now,
	})
}
