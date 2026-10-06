// Package evidence evaluates typed observations without executing operations.
package evidence

import (
	"fmt"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/contract"
)

type Policy struct {
	MaxBackupAge time.Duration
	StartupGrace time.Duration
}

// Evaluate uses current evidence only. The store preserves unresolved findings
// when collection fails instead of evaluating a missing observation as recovery.
func Evaluate(deployment *contract.Deployment, createdAt time.Time, paused bool, policy Policy, now time.Time) {
	deployment.Issues = []contract.Issue{}
	add := func(rule string, severity contract.IssueSeverity, category contract.IssueCategory, title, detail string) {
		deployment.Issues = append(deployment.Issues, contract.Issue{
			Id: deployment.Id + ":" + rule, DeploymentId: deployment.Id, Rule: rule,
			Severity: severity, Category: category, Title: title, Detail: detail,
			FirstSeenAt: now, LastCheckedAt: now,
		})
	}
	deployment.Health = "unknown"
	if deployment.ReadyReplicas != nil && deployment.DesiredReplicas != nil && *deployment.DesiredReplicas > 0 {
		if *deployment.ReadyReplicas >= *deployment.DesiredReplicas {
			deployment.Health = "healthy"
		}
		if !paused && now.Sub(createdAt) >= policy.StartupGrace && *deployment.ReadyReplicas < *deployment.DesiredReplicas {
			severity := contract.IssueSeverityHigh
			if *deployment.ReadyReplicas == 0 {
				severity = "critical"
			}
			add("replica-readiness", severity, "availability", "Replicas need attention", fmt.Sprintf("%d of %d replicas are ready. This is workload readiness evidence, not a client transaction check.", *deployment.ReadyReplicas, *deployment.DesiredReplicas))
		}
	}
	for _, condition := range deployment.Conditions {
		if condition.ObservedGeneration < deployment.Generation {
			continue
		}
		if !paused && condition.Type == "OpenBaoSealed" && condition.Status == "True" {
			add("sealed", "critical", "availability", "OpenBao is sealed", "The operator reports a sealed deployment. Investigate unseal access and current workload evidence.")
		}
	}
	if failure := deployment.UpgradeFailure; failure != nil {
		add("upgrade-failed", "high", "operations", "Upgrade requires investigation", "The operator records a failed upgrade with reason "+failure.Reason+". Inspect its execution evidence.")
	}
	if backup := deployment.Backup; backup != nil {
		if backup.ConsecutiveFailures > 0 {
			add("backup-failed", "high", "backup", "Backup failed", fmt.Sprintf("%d consecutive backup attempts failed. Last success and latest failure are separate evidence.", backup.ConsecutiveFailures))
		} else if backup.LastSuccessAt == nil {
			add("backup-never-succeeded", "high", "backup", "No successful backup observed", "Backups are configured, but no successful completion is recorded.")
		} else if now.Sub(*backup.LastSuccessAt) > policy.MaxBackupAge {
			add("backup-overdue", "high", "backup", "Backup is overdue", fmt.Sprintf("Last successful completion exceeds the configured %.0f-hour threshold. Completion age is not measured RPO.", policy.MaxBackupAge.Hours()))
		}
		if test := backup.RestoreTest; test != nil && (test.Outcome == "failed" || test.Outcome == "blocked") {
			add("restore-test", "high", "backup", "Restore test needs attention", "The latest restore test failed or is blocked. Its result covers the pinned snapshot and disposable target only.")
		}
	}
	if replica := deployment.Replica; replica != nil && replica.HistoryConflict {
		add("replica-history-conflict", "critical", "operations", "Replica history conflict", "The remote replica observation records conflicting histories. An applied index does not authorize survivor selection.")
	}
	if recovery := deployment.Recovery; recovery != nil && (recovery.Phase == "unknown" || recovery.Phase == "failed" || recovery.Cleanup == "failed") {
		add("recovery-investigate", "high", "operations", "Recovery requires investigation", "Execution or cleanup requires investigation. A recorded submission claim never authorizes replay.")
	}
	for _, issue := range deployment.Issues {
		if issue.Severity == "critical" {
			deployment.Health = "critical"
			break
		}
		if issue.Severity == "high" {
			deployment.Health = "attention"
		}
	}
}

func CollectionFailure(deployment *contract.Deployment, state contract.EvidenceCollection, now time.Time) {
	deployment.Evidence.Collection = state
	deployment.Evidence.LastAttemptAt = now
	for i := range deployment.Issues {
		if deployment.Issues[i].Rule == "collection" {
			deployment.Issues[i].LastCheckedAt = now
			return
		}
	}
	deployment.Issues = append(deployment.Issues, contract.Issue{
		Id: deployment.Id + ":collection", DeploymentId: deployment.Id, Rule: "collection",
		Severity: "high", Category: "visibility", Title: "Collection is incomplete",
		Detail:      "The connector cannot refresh this deployment. Workload state and unresolved findings are last known.",
		FirstSeenAt: now, LastCheckedAt: now,
	})
}
