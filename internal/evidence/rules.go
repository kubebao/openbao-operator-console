// Package evidence evaluates typed observations without executing operations.
package evidence

import (
	"fmt"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// subject is the evidence one evaluation reads.
type subject struct {
	deployment fleet.Deployment
	createdAt  time.Time
	paused     bool
	policy     fleet.Policy
	now        time.Time
}

// finding is an issue before it is bound to a deployment and evaluation time.
type finding struct {
	rule     fleet.Rule
	severity fleet.Severity
	category fleet.Category
	title    string
	detail   string
}

// rules run in order; each reports at most one finding.
var rules = []func(subject) *finding{
	replicaReadiness,
	sealed,
	upgradeFailed,
	backupAge,
	restoreTest,
	replicaHistoryConflict,
	recoveryInvestigation,
}

// Evaluate uses current evidence only. The store preserves unresolved findings
// when collection fails instead of evaluating a missing observation as recovery.
func Evaluate(deployment *fleet.Deployment, createdAt time.Time, paused bool, policy fleet.Policy, now time.Time) {
	input := subject{deployment: *deployment, createdAt: createdAt, paused: paused, policy: policy, now: now}
	deployment.Issues = []fleet.Issue{}
	for _, rule := range rules {
		if f := rule(input); f != nil {
			deployment.Issues = append(deployment.Issues, fleet.Issue{
				ID:            deployment.ID + ":" + string(f.rule),
				DeploymentID:  deployment.ID,
				Rule:          f.rule,
				Severity:      f.severity,
				Category:      f.category,
				Title:         f.title,
				Detail:        f.detail,
				FirstSeenAt:   now,
				LastCheckedAt: now,
			})
		}
	}
	deployment.Health = health(*deployment)
}

// health is the most severe finding, or readiness when no finding applies.
func health(deployment fleet.Deployment) fleet.Health {
	result := fleet.HealthUnknown
	if ready, desired := deployment.ReadyReplicas, deployment.DesiredReplicas; ready != nil && desired != nil && *desired > 0 &&
		*ready >= *desired {
		result = fleet.HealthHealthy
	}
	for _, issue := range deployment.Issues {
		switch issue.Severity {
		case fleet.SeverityCritical:
			return fleet.HealthCritical
		case fleet.SeverityHigh:
			result = fleet.HealthAttention
		case fleet.SeverityInfo:
		}
	}
	return result
}

// A paused deployment or one still inside its startup grace is not expected to be ready.
func replicaReadiness(s subject) *finding {
	ready, desired := s.deployment.ReadyReplicas, s.deployment.DesiredReplicas
	if ready == nil || desired == nil || *desired == 0 || *ready >= *desired || s.paused ||
		s.now.Sub(s.createdAt) < s.policy.StartupGrace {
		return nil
	}
	severity := fleet.SeverityHigh
	if *ready == 0 {
		severity = fleet.SeverityCritical
	}
	return &finding{
		rule:     fleet.RuleReplicaReadiness,
		severity: severity,
		category: fleet.CategoryAvailability,
		title:    "Replicas need attention",
		detail: fmt.Sprintf(
			"%d of %d replicas are ready. This is workload readiness evidence, not a client transaction check.",
			*ready,
			*desired,
		),
	}
}

// Only a condition observed for the current generation counts.
func sealed(s subject) *finding {
	if s.paused {
		return nil
	}
	for _, condition := range s.deployment.Conditions {
		if condition.ObservedGeneration >= s.deployment.Generation && condition.Type == "OpenBaoSealed" &&
			condition.Status == fleet.ConditionStatusTrue {
			return &finding{
				rule:     fleet.RuleSealed,
				severity: fleet.SeverityCritical,
				category: fleet.CategoryAvailability,
				title:    "OpenBao is sealed",
				detail:   "The operator reports a sealed deployment. Investigate unseal access and current workload evidence.",
			}
		}
	}
	return nil
}

func upgradeFailed(s subject) *finding {
	failure := s.deployment.UpgradeFailure
	if failure == nil {
		return nil
	}
	return &finding{
		rule:     fleet.RuleUpgradeFailed,
		severity: fleet.SeverityHigh,
		category: fleet.CategoryOperations,
		title:    "Upgrade requires investigation",
		detail:   "The operator records a failed upgrade with reason " + failure.Reason + ". Inspect its execution evidence.",
	}
}

// backupAge reports the first of: recent failures, no success, or an overdue success.
func backupAge(s subject) *finding {
	backup := s.deployment.Backup
	switch {
	case backup == nil:
		return nil
	case backup.ConsecutiveFailures > 0:
		return &finding{
			rule:     fleet.RuleBackupFailed,
			severity: fleet.SeverityHigh,
			category: fleet.CategoryBackup,
			title:    "Backup failed",
			detail: fmt.Sprintf(
				"%d consecutive backup attempts failed. Last success and latest failure are separate evidence.",
				backup.ConsecutiveFailures,
			),
		}
	case backup.LastSuccessAt == nil:
		return &finding{
			rule:     fleet.RuleBackupNeverSucceeded,
			severity: fleet.SeverityHigh,
			category: fleet.CategoryBackup,
			title:    "No successful backup observed",
			detail:   "Backups are configured, but no successful completion is recorded.",
		}
	case s.now.Sub(*backup.LastSuccessAt) > s.policy.MaxBackupAge:
		return &finding{
			rule:     fleet.RuleBackupOverdue,
			severity: fleet.SeverityHigh,
			category: fleet.CategoryBackup,
			title:    "Backup is overdue",
			detail: fmt.Sprintf(
				"Last successful completion exceeds the configured %.0f-hour threshold. Completion age is not measured RPO.",
				s.policy.MaxBackupAge.Hours(),
			),
		}
	default:
		return nil
	}
}

func restoreTest(s subject) *finding {
	if s.deployment.Backup == nil || s.deployment.Backup.RestoreTest == nil {
		return nil
	}
	if outcome := s.deployment.Backup.RestoreTest.Outcome; outcome != fleet.RestoreOutcomeFailed && outcome != fleet.RestoreOutcomeBlocked {
		return nil
	}
	return &finding{
		rule:     fleet.RuleRestoreTest,
		severity: fleet.SeverityHigh,
		category: fleet.CategoryBackup,
		title:    "Restore test needs attention",
		detail:   "The latest restore test failed or is blocked. Its result covers the pinned snapshot and disposable target only.",
	}
}

func replicaHistoryConflict(s subject) *finding {
	if s.deployment.Replica == nil || !s.deployment.Replica.HistoryConflict {
		return nil
	}
	return &finding{
		rule:     fleet.RuleReplicaHistoryConflict,
		severity: fleet.SeverityCritical,
		category: fleet.CategoryOperations,
		title:    "Replica history conflict",
		detail:   "The remote replica observation records conflicting histories. An applied index does not authorize survivor selection.",
	}
}

func recoveryInvestigation(s subject) *finding {
	recovery := s.deployment.Recovery
	if recovery == nil || (recovery.Phase != fleet.RecoveryPhaseUnknown && recovery.Phase != fleet.RecoveryPhaseFailed &&
		recovery.Cleanup != fleet.RecoveryCleanupFailed) {
		return nil
	}
	return &finding{
		rule:     fleet.RuleRecoveryInvestigate,
		severity: fleet.SeverityHigh,
		category: fleet.CategoryOperations,
		title:    "Recovery requires investigation",
		detail:   "Execution or cleanup requires investigation. A recorded submission claim never authorizes replay.",
	}
}

// CollectionFailure records a failed collection attempt. Workload facts and unresolved
// findings remain last known.
func CollectionFailure(deployment *fleet.Deployment, state fleet.EvidenceCollection, attemptedAt time.Time) {
	deployment.Evidence.LastAttemptAt = attemptedAt
	incomplete(deployment, state, attemptedAt, attemptedAt)
}

// Stale marks evidence that has not been refreshed since it became stale. It records no
// collection attempt: the last attempt time stays the time of the last real attempt.
func Stale(deployment *fleet.Deployment, since, now time.Time) {
	incomplete(deployment, fleet.EvidenceCollectionStale, since, now)
}

// incomplete adds or refreshes the collection finding and recomputes health, so a
// deployment is never healthy while its evidence is not current. Critical findings
// remain critical.
func incomplete(deployment *fleet.Deployment, state fleet.EvidenceCollection, since, checked time.Time) {
	deployment.Evidence.Collection = state
	defer func() { deployment.Health = health(*deployment) }()
	for i := range deployment.Issues {
		if deployment.Issues[i].Rule == fleet.RuleCollection {
			deployment.Issues[i].LastCheckedAt = checked
			return
		}
	}
	deployment.Issues = append(deployment.Issues, fleet.Issue{
		ID:            deployment.ID + ":" + string(fleet.RuleCollection),
		DeploymentID:  deployment.ID,
		Rule:          fleet.RuleCollection,
		Severity:      fleet.SeverityHigh,
		Category:      fleet.CategoryVisibility,
		Title:         "Collection is incomplete",
		Detail:        "The connector cannot refresh this deployment. Workload state and unresolved findings are last known.",
		FirstSeenAt:   since,
		LastCheckedAt: checked,
	})
}
