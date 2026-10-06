// Package store retains approved observations and durable issue first-seen times.
package store

import (
	"context"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/contract"
)

type Batch struct {
	Connection  contract.Connection
	Deployments []contract.Deployment
}

type Repository interface {
	Apply(context.Context, Batch) error
	Snapshot(context.Context) (contract.Snapshot, error)
	History(context.Context, string) ([]contract.Observation, error)
	Close()
}

// PreserveOperatorEvidence retains last-known facts after an independent read fails.
// Identity and namespace assignments always come from the current enrollment.
func PreserveOperatorEvidence(current *contract.Connection, previous contract.Connection) {
	for i := range current.Operators {
		next := &current.Operators[i]
		if next.Collection == "current" || next.Collection == "invalid" {
			continue
		}
		for _, old := range previous.Operators {
			if old.Id != next.Id || old.Namespace != next.Namespace || old.Name != next.Name || old.Container != next.Container {
				continue
			}
			next.Uid, next.Image, next.Version = old.Uid, old.Image, old.Version
			next.Health, next.Generation, next.ObservedGeneration = old.Health, old.Generation, old.ObservedGeneration
			next.ReadyReplicas, next.DesiredReplicas, next.UpdatedReplicas = old.ReadyReplicas, old.DesiredReplicas, old.UpdatedReplicas
			next.LastSuccessAt = old.LastSuccessAt
		}
	}
}

func PreserveFirstSeen(current *contract.Deployment, previous contract.Deployment) {
	for i := range current.Issues {
		for _, old := range previous.Issues {
			if current.Issues[i].Id == old.Id {
				current.Issues[i].FirstSeenAt = old.FirstSeenAt
				break
			}
		}
	}
}

func Transition(current, previous contract.Deployment, now time.Time) *contract.Activity {
	if current.Health == previous.Health && current.Evidence.Collection == previous.Evidence.Collection && SameFindings(current, previous) {
		return nil
	}
	outcome := contract.ActivityOutcomeInfo
	if current.Health == "healthy" && current.Evidence.Collection == "current" {
		outcome = "success"
	}
	if current.Health == "critical" {
		outcome = "error"
	}
	return &contract.Activity{Id: current.Id + ":" + now.Format(time.RFC3339Nano), DeploymentId: current.Id,
		At: now, Title: "Observed state changed", Detail: string(previous.Health) + " → " + string(current.Health) + "; collection " + string(current.Evidence.Collection), Outcome: outcome}
}

// SameFindings ignores polling timestamps and compares the investigation state.
func SameFindings(current, previous contract.Deployment) bool {
	if len(current.Issues) != len(previous.Issues) {
		return false
	}
	old := make(map[string]contract.Issue, len(previous.Issues))
	for _, issue := range previous.Issues {
		old[issue.Id] = issue
	}
	for _, issue := range current.Issues {
		before, exists := old[issue.Id]
		if !exists || issue.Severity != before.Severity || issue.Detail != before.Detail {
			return false
		}
	}
	return true
}
