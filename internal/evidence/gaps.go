package evidence

import (
	"slices"
	"time"

	"github.com/kubebao/openbao-operator-client/diagnose"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// appendGap combines related-subject coverage by rule and read outcome. Subject identity and raw text stay excluded.
func appendGap(evidence *fleet.Evidence, rule fleet.Rule, coverage diagnose.Coverage, at time.Time) {
	outcome := fleet.DiagnosticRead("unknown")
	if coverage.Outcome.Valid() {
		outcome = fleet.DiagnosticRead(coverage.Outcome)
	}
	index := slices.IndexFunc(evidence.AnalysisGaps, func(gap fleet.DiagnosticGap) bool {
		return gap.Rule == rule && gap.CollectionOutcome == outcome
	})
	if index == -1 {
		index = len(evidence.AnalysisGaps)
		evidence.AnalysisGaps = append(evidence.AnalysisGaps, fleet.DiagnosticGap{
			Rule: rule, CollectionOutcome: outcome, CheckedAt: at,
		})
	}
	reasons := coverage.Reasons
	if len(reasons) == 0 {
		reasons = []diagnose.CoverageReason{coverage.Reason}
	}
	gap := &evidence.AnalysisGaps[index]
	for _, reason := range reasons {
		safe := gapReason(reason)
		if !slices.Contains(gap.Reasons, safe) {
			gap.Reasons = append(gap.Reasons, safe)
		}
	}
	slices.Sort(gap.Reasons)
}

func gapReason(reason diagnose.CoverageReason) fleet.DiagnosticReason {
	switch reason {
	case diagnose.ReasonExcluded, diagnose.ReasonNotRequested, diagnose.ReasonScopeIncomplete, diagnose.ReasonMissingObservation,
		diagnose.ReasonStaleEvidence, diagnose.ReasonMissingField, diagnose.ReasonGeneration, diagnose.ReasonTime,
		diagnose.ReasonNotConfigured, diagnose.ReasonHigherPriority, diagnose.ReasonVerification, diagnose.ReasonAssociation,
		diagnose.ReasonScopeRestricted:
		return fleet.DiagnosticReason(reason)
	default:
		return "unknown"
	}
}
