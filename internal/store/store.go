// Package store retains approved observations and durable issue first-seen times.
package store

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type Batch struct {
	Connection  fleet.Connection
	Deployments []fleet.Deployment
}

type Repository interface {
	Apply(context.Context, Batch) error
	Snapshot(context.Context) (fleet.Snapshot, error)
	History(context.Context, string) ([]fleet.Observation, error)
	Close()
}

// PreserveOperatorEvidence retains last-known facts after an independent read fails.
// Identity and namespace assignments always come from the current enrollment.
func PreserveOperatorEvidence(current *fleet.Connection, previous fleet.Connection) {
	for i := range current.Operators {
		next := &current.Operators[i]
		if next.Collection == fleet.OperatorCollectionCurrent || next.Collection == fleet.OperatorCollectionInvalid {
			continue
		}
		for _, old := range previous.Operators {
			if old.ID != next.ID || old.Namespace != next.Namespace || old.Name != next.Name || old.Container != next.Container {
				continue
			}
			next.UID, next.Image, next.Version = old.UID, old.Image, old.Version
			next.Health, next.Generation, next.ObservedGeneration = old.Health, old.Generation, old.ObservedGeneration
			next.ReadyReplicas, next.DesiredReplicas, next.UpdatedReplicas = old.ReadyReplicas, old.DesiredReplicas, old.UpdatedReplicas
			next.LastSuccessAt = old.LastSuccessAt
		}
	}
}

func PreserveFirstSeen(current *fleet.Deployment, previous fleet.Deployment) {
	for i := range current.Issues {
		for _, old := range previous.Issues {
			if current.Issues[i].ID == old.ID {
				current.Issues[i].FirstSeenAt = old.FirstSeenAt
				break
			}
		}
	}
}

func Transition(current, previous fleet.Deployment, now time.Time) *fleet.Activity {
	if current.Health == previous.Health && current.Evidence.Collection == previous.Evidence.Collection && SameFindings(current, previous) {
		return nil
	}
	outcome := fleet.ActivityOutcomeInfo
	if current.Health == fleet.HealthHealthy && current.Evidence.Collection == fleet.EvidenceCollectionCurrent {
		outcome = fleet.ActivityOutcomeSuccess
	}
	if current.Health == fleet.HealthCritical {
		outcome = fleet.ActivityOutcomeError
	}
	return &fleet.Activity{
		ID:           current.ID + ":" + now.Format(time.RFC3339Nano),
		DeploymentID: current.ID,
		At:           now,
		Title:        "Observed state changed",
		Detail:       string(previous.Health) + " → " + string(current.Health) + "; collection " + string(current.Evidence.Collection),
		Outcome:      outcome,
	}
}

// SameFindings ignores polling timestamps and compares the investigation state.
func SameFindings(current, previous fleet.Deployment) bool {
	if len(current.Issues) != len(previous.Issues) {
		return false
	}
	old := make(map[string]fleet.Issue, len(previous.Issues))
	for _, issue := range previous.Issues {
		old[issue.ID] = issue
	}
	for _, issue := range current.Issues {
		before, exists := old[issue.ID]
		if !exists || issue.Severity != before.Severity || issue.Detail != before.Detail {
			return false
		}
	}
	return true
}

// volatileFields change on every operator status write or collection round without changing
// what the console shows. Observations are recorded only when other fields change, so the
// retained history describes meaningful changes instead of resource version churn.
var volatileFields = map[string][]string{
	"":         {"resourceVersion"},
	"evidence": {"collectedAt", "lastAttemptAt", "producerObservedAt"},
	"replica":  {"observedAt", "localIndex", "sourceIndex"},
}

func displayedFields(deployment fleet.Deployment) (map[string]any, error) {
	encoded, err := json.Marshal(deployment)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return nil, err
	}
	for parent, fields := range volatileFields {
		target := value
		if parent != "" {
			nested, ok := value[parent].(map[string]any)
			if !ok {
				continue
			}
			target = nested
		}
		for _, field := range fields {
			delete(target, field)
		}
	}
	if issues, ok := value["issues"].([]any); ok {
		for _, raw := range issues {
			if issue, ok := raw.(map[string]any); ok {
				delete(issue, "firstSeenAt")
				delete(issue, "lastCheckedAt")
			}
		}
	}
	return value, nil
}

// ObservationChanged reports whether a deployment differs from its previous projection in a
// field the console displays or evaluates. A projection that cannot be compared counts as changed.
func ObservationChanged(current, previous fleet.Deployment) bool {
	a, err := displayedFields(current)
	if err != nil {
		return true
	}
	b, err := displayedFields(previous)
	if err != nil {
		return true
	}
	return !reflect.DeepEqual(a, b)
}
