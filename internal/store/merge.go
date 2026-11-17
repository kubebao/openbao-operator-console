package store

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// changes are the writes that one collection batch produces.
type changes struct {
	connection   fleet.Connection
	deployments  []fleet.Deployment
	observations []fleet.Deployment
	activity     []fleet.Activity
	// complete reports a successful collection. Stored deployments absent from it no longer exist.
	complete   bool
	recordedAt time.Time
}

// merge combines a batch with the stored state of its connection. A failed collection
// keeps every stored deployment as last known instead of treating missing evidence as
// recovery. History records only changes the console displays.
func merge(batch Batch, previous *fleet.Connection, stored []fleet.Deployment) changes {
	result := changes{connection: batch.Connection, complete: batch.Connection.Collection == fleet.ConnectionCollectionCurrent}
	if previous != nil {
		if !result.complete {
			result.connection.LastSuccessAt = previous.LastSuccessAt
		}
		preserveOperatorEvidence(&result.connection, *previous)
	}
	byID := make(map[string]fleet.Deployment, len(stored))
	for _, deployment := range stored {
		byID[deployment.ID] = deployment
	}
	deployments := batch.Deployments
	if !result.complete {
		deployments = lastKnown(&result.connection, stored)
	}
	at := batch.CompletedAt
	if at.IsZero() {
		at = result.connection.LastAttemptAt
	}
	result.recordedAt = at
	for _, current := range deployments {
		old, exists := byID[current.ID]
		if exists {
			retainIncomplete(&current, old, batch.Unresolved[current.ID])
			preserveFirstSeen(&current, old)
		}
		result.deployments = append(result.deployments, current)
		if exists && !observationChanged(current, old) {
			continue
		}
		result.observations = append(result.observations, current)
		if exists {
			result.activity = append(result.activity, evidence.Transitions(old, current, at)...)
		}
	}
	return result
}

// lastKnown marks every stored deployment with the failed collection state. The
// connection's last success becomes the newest retained collection time.
func lastKnown(connection *fleet.Connection, stored []fleet.Deployment) []fleet.Deployment {
	result := make([]fleet.Deployment, 0, len(stored))
	for _, deployment := range stored {
		evidence.CollectionFailure(&deployment, failedEvidence(connection.Collection), connection.LastAttemptAt)
		if connection.LastSuccessAt == nil || deployment.Evidence.CollectedAt.After(*connection.LastSuccessAt) {
			collected := deployment.Evidence.CollectedAt
			connection.LastSuccessAt = &collected
		}
		result = append(result, deployment)
	}
	return result
}

func failedEvidence(collection fleet.ConnectionCollection) fleet.EvidenceCollection {
	switch collection {
	case fleet.ConnectionCollectionDenied:
		return fleet.EvidenceCollectionDenied
	case fleet.ConnectionCollectionUnsupported:
		return fleet.EvidenceCollectionUnsupported
	case fleet.ConnectionCollectionStale:
		return fleet.EvidenceCollectionStale
	default:
		return fleet.EvidenceCollectionUnreachable
	}
}

// preserveOperatorEvidence retains last-known facts after an independent read fails.
// Identity and namespace assignments always come from the current enrollment.
func preserveOperatorEvidence(current *fleet.Connection, previous fleet.Connection) {
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

// retainIncomplete preserves an unresolved episode without advancing its last
// confirmed evaluation time. A successful inventory read can still have denied
// optional reads or unknown producer fields.
func retainIncomplete(current *fleet.Deployment, previous fleet.Deployment, rules []fleet.Rule) {
	if len(rules) == 0 {
		return
	}
	for _, old := range previous.Issues {
		if !slices.Contains(rules, old.Rule) {
			continue
		}
		found := slices.ContainsFunc(current.Issues, func(next fleet.Issue) bool { return next.ID == old.ID })
		if !found {
			current.Issues = append(current.Issues, old)
		}
	}
	if current.Recovery == nil && slices.Contains(rules, fleet.RuleRecoveryInvestigate) {
		current.Recovery = previous.Recovery
	}
	evidence.RefreshHealth(current)
}

func preserveFirstSeen(current *fleet.Deployment, previous fleet.Deployment) {
	for i := range current.Issues {
		for _, old := range previous.Issues {
			if current.Issues[i].ID == old.ID {
				current.Issues[i].FirstSeenAt = old.FirstSeenAt
				break
			}
		}
	}
}

// displayed clears fields that change on every operator status write or collection round
// without changing what the console shows.
func displayed(deployment fleet.Deployment) fleet.Deployment {
	deployment.ResourceVersion = ""
	deployment.Evidence.CollectedAt, deployment.Evidence.LastAttemptAt = time.Time{}, time.Time{}
	deployment.Evidence.ProducerObservedAt = nil
	deployment.Evidence.AnalysisGaps = slices.Clone(deployment.Evidence.AnalysisGaps)
	for i := range deployment.Evidence.AnalysisGaps {
		deployment.Evidence.AnalysisGaps[i].CheckedAt = time.Time{}
	}
	if deployment.Replica != nil {
		replica := *deployment.Replica
		replica.ObservedAt, replica.LocalIndex, replica.SourceIndex = nil, nil, nil
		deployment.Replica = &replica
	}
	deployment.Issues = slices.Clone(deployment.Issues)
	for i := range deployment.Issues {
		deployment.Issues[i].FirstSeenAt, deployment.Issues[i].LastCheckedAt = time.Time{}, time.Time{}
	}
	return deployment
}

// observationChanged reports whether a deployment differs from its previous projection in a
// field the console displays or evaluates. A projection that cannot be compared counts as changed.
func observationChanged(current, previous fleet.Deployment) bool {
	a, err := json.Marshal(displayed(current))
	if err != nil {
		return true
	}
	b, err := json.Marshal(displayed(previous))
	return err != nil || !bytes.Equal(a, b)
}
