// Package evidence maps shared diagnostics into console health, findings, and history.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/kubebao/openbao-operator-client/diagnose"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type ruleMapping struct {
	shared   diagnose.RuleID
	local    fleet.Rule
	category fleet.Category
	title    string
}

var mappings = []ruleMapping{
	{diagnose.RuleReadiness, fleet.RuleReplicaReadiness, fleet.CategoryAvailability, "Replicas need attention"},
	{diagnose.RuleSealed, fleet.RuleSealed, fleet.CategoryAvailability, "OpenBao is sealed"},
	{diagnose.RuleUpgrade, fleet.RuleUpgradeFailed, fleet.CategoryOperations, "Upgrade requires investigation"},
	{diagnose.RuleBackupFailed, fleet.RuleBackupFailed, fleet.CategoryBackup, "Backup failed"},
	{diagnose.RuleBackupAbsent, fleet.RuleBackupNeverSucceeded, fleet.CategoryBackup, "No successful backup observed"},
	{diagnose.RuleBackupAge, fleet.RuleBackupOverdue, fleet.CategoryBackup, "Backup is overdue"},
	{diagnose.RuleRestoreTest, fleet.RuleRestoreTest, fleet.CategoryBackup, "Restore test needs attention"},
	{diagnose.RuleRestore, fleet.RuleRecoveryInvestigate, fleet.CategoryOperations, "Recovery requires investigation"},
}

// Rules preserves the console's selected diagnostic coverage without adding workload reads.
func Rules() []diagnose.RuleID {
	result := make([]diagnose.RuleID, 0, len(mappings))
	for _, m := range mappings {
		result = append(result, m.shared)
	}
	return result
}

// Apply maps already evaluated findings. Missing evaluation remains distinguishable
// from a confirmed resolution; the store retains affected prior issues by ID.
func Apply(deployment *fleet.Deployment, report diagnose.Report) []fleet.Rule {
	deployment.Issues = []fleet.Issue{}
	deployment.Evidence.AnalysisIncomplete = false
	unresolved := []fleet.Rule{}
	for _, m := range mappings {
		for _, c := range report.Coverage {
			if c.Rule == m.shared && c.State == diagnose.Insufficient {
				deployment.Evidence.AnalysisIncomplete = true
				unresolved = append(unresolved, m.local)
			}
		}
		for _, f := range report.Findings {
			if f.Rule != m.shared {
				continue
			}
			severity := fleet.SeverityHigh
			switch f.Severity {
			case diagnose.Critical:
				severity = fleet.SeverityCritical
			case diagnose.Info:
				severity = fleet.SeverityInfo
			}
			id := deployment.ID + ":" + string(m.local)
			detail := f.Summary
			if f.Subject.Kind == "OpenBaoRestore" {
				key, err := f.Key()
				if err != nil {
					continue
				}
				sum := sha256.Sum256([]byte(key))
				id += ":" + hex.EncodeToString(sum[:16])
				detail = "Restore " + f.Subject.Name + ": " + detail
			}
			deployment.Issues = append(deployment.Issues, fleet.Issue{
				ID: id, DeploymentID: deployment.ID, Rule: m.local,
				Severity: severity, Category: m.category, Title: m.title, Detail: detail,
				FirstSeenAt: report.Policy.ReferenceTime, LastCheckedAt: report.Policy.ReferenceTime,
			})
		}
	}
	deployment.Health = health(*deployment)
	return unresolved
}

// RefreshHealth recomputes the product summary after retaining unresolved findings.
func RefreshHealth(deployment *fleet.Deployment) { deployment.Health = health(*deployment) }

// health is the most severe finding, or readiness when no finding applies.
func health(deployment fleet.Deployment) fleet.Health {
	result := fleet.HealthUnknown
	if ready, desired := deployment.ReadyReplicas, deployment.DesiredReplicas; !deployment.Evidence.AnalysisIncomplete && ready != nil &&
		desired != nil &&
		*desired > 0 &&
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
