package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

func TestDeploymentContractKeepsAcknowledgementAndDiagnosisCauses(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	observed := int64(2)
	d := fleet.Deployment{ObservedGeneration: &observed, Evidence: fleet.Evidence{
		AnalysisIncomplete: true, AnalysisGaps: []fleet.DiagnosticGap{{
			Rule:    fleet.RuleRecoveryInvestigate,
			Reasons: []fleet.DiagnosticReason{"scope_incomplete"}, CollectionOutcome: "denied", CheckedAt: now,
		}},
	}}
	got := deploymentContract(d)
	if got.ObservedGeneration == nil || *got.ObservedGeneration != 2 || got.Evidence.AnalysisGaps == nil ||
		len(*got.Evidence.AnalysisGaps) != 1 {
		t.Fatalf("contract dropped diagnosis: %+v", got)
	}
	gap := (*got.Evidence.AnalysisGaps)[0]
	if gap.CollectionOutcome != "denied" || len(gap.Reasons) != 1 || gap.Reasons[0] != "scope_incomplete" || !gap.CheckedAt.Equal(now) {
		t.Fatalf("contract changed cause or time: %+v", gap)
	}
	older := deploymentContract(fleet.Deployment{Evidence: fleet.Evidence{AnalysisIncomplete: true}})
	if older.ObservedGeneration != nil || older.Evidence.AnalysisGaps != nil {
		t.Fatal("older evidence was filled in")
	}
	encoded, err := json.Marshal(older)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["observedGeneration"]) != "null" {
		t.Fatal("missing acknowledgement is not represented as null")
	}
}
