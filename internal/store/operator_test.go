package store

import (
	"testing"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/contract"
)

func TestOperatorFailurePreservesFactsWithoutRestoringAssignments(t *testing.T) {
	version, uid := "0.5.1", "observed-uid"
	now := time.Now()
	previous := contract.Connection{
		Operators: []contract.Operator{
			{
				Id:            "one",
				Namespace:     "system",
				Name:          "controller",
				Container:     "manager",
				Namespaces:    []string{"old-team"},
				Collection:    "current",
				Health:        "available",
				Uid:           &uid,
				Version:       &version,
				LastSuccessAt: &now,
			},
		},
	}
	current := contract.Connection{
		Operators: []contract.Operator{
			{
				Id:            "one",
				Namespace:     "system",
				Name:          "controller",
				Container:     "manager",
				Namespaces:    []string{"team"},
				Collection:    "denied",
				LastAttemptAt: now.Add(time.Minute),
			},
		},
	}
	PreserveOperatorEvidence(&current, previous)
	result := current.Operators[0]
	if result.Collection != "denied" || result.Version == nil || result.LastSuccessAt == nil || result.Namespaces[0] != "team" {
		t.Fatal("failed read lost last-known facts or revived old assignments")
	}
	current.Operators[0].Name, current.Operators[0].Version = "replacement", nil
	PreserveOperatorEvidence(&current, previous)
	if current.Operators[0].Version != nil {
		t.Fatal("new enrolled workload inherited old evidence")
	}
}
