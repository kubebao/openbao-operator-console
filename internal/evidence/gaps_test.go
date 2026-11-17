package evidence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestDiagnosticGapsPreserveSafeCausesAndClearAfterEvaluation(t *testing.T) {
	now := time.Now().UTC()
	d := fleet.Deployment{ID: "test"}
	report := diagnose.Report{Policy: diagnose.Policy{ReferenceTime: now}, Coverage: []diagnose.Coverage{
		{
			Rule: diagnose.RuleRestore, State: diagnose.Insufficient, Outcome: shared.CollectionDenied,
			Reasons: []diagnose.CoverageReason{diagnose.ReasonScopeIncomplete, diagnose.ReasonGeneration},
		},
		{
			Rule: diagnose.RuleRestore, State: diagnose.Insufficient, Outcome: shared.CollectionDenied,
			Reason: diagnose.ReasonMissingField,
		},
		{
			Rule: diagnose.RuleRestore, State: diagnose.Insufficient, Outcome: shared.CollectionDenied,
			Reason: diagnose.ReasonMissingField,
		},
		{
			Rule: diagnose.RuleSealed, State: diagnose.Insufficient, Outcome: "PRIVATE_GAP_CANARY", Reason: "PRIVATE_GAP_CANARY",
			Subject: shared.Identity{Name: "PRIVATE_GAP_CANARY"},
		},
	}}
	Apply(&d, report)
	if !d.Evidence.AnalysisIncomplete || len(d.Evidence.AnalysisGaps) != 2 {
		t.Fatalf("gaps lost: %+v", d.Evidence)
	}
	gap := d.Evidence.AnalysisGaps[0]
	if gap.Rule != fleet.RuleRecoveryInvestigate || gap.CollectionOutcome != "denied" || !gap.CheckedAt.Equal(now) ||
		len(gap.Reasons) != 3 || gap.Reasons[0] != "generation_unconfirmed" || gap.Reasons[1] != "missing_field" ||
		gap.Reasons[2] != "scope_incomplete" {
		t.Fatalf("gap aggregation lost causes or time: %+v", gap)
	}
	encoded, err := json.Marshal(d.Evidence)
	if err != nil || strings.Contains(string(encoded), "PRIVATE_GAP_CANARY") {
		t.Fatalf("unapproved coverage text escaped: %s %v", encoded, err)
	}
	if gap := d.Evidence.AnalysisGaps[1]; gap.CollectionOutcome != "unknown" || len(gap.Reasons) != 1 || gap.Reasons[0] != "unknown" {
		t.Fatalf("unrecognized codes did not remain unknown: %+v", gap)
	}
	CollectionFailure(&d, fleet.EvidenceCollectionUnreachable, now.Add(time.Minute))
	if !d.Evidence.AnalysisGaps[0].CheckedAt.Equal(now) {
		t.Fatal("failed collection advanced analysis time")
	}
	Apply(&d, diagnose.Report{
		Policy:   diagnose.Policy{ReferenceTime: now.Add(2 * time.Minute)},
		Coverage: []diagnose.Coverage{{Rule: diagnose.RuleRestore, State: diagnose.Evaluated}},
	})
	if d.Evidence.AnalysisIncomplete || len(d.Evidence.AnalysisGaps) != 0 {
		t.Fatal("completed analysis retained old gaps")
	}
}
