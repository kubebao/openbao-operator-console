package httpapi

import (
	"time"

	"github.com/kubebao/openbao-operator-console/internal/buildinfo"
	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/contract"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// The functions in this file project the fleet domain model onto the generated API
// contract. They are the only place where fleet types become HTTP output.

func mapSlice[T, R any](items []T, project func(T) R) []R {
	result := make([]R, 0, len(items))
	for _, item := range items {
		result = append(result, project(item))
	}
	return result
}

func snapshotContract(snapshot fleet.Snapshot, user config.User, demo bool) contract.Snapshot {
	version, commit := buildinfo.Current()
	mode := contract.Live
	if demo {
		mode = contract.Demo
	}
	viewer := contract.Viewer{Name: user.Name, Subject: user.Subject, Admin: user.Admin, ConnectionIds: []string{}}
	for _, connection := range snapshot.Connections {
		viewer.ConnectionIds = append(viewer.ConnectionIds, connection.ID)
	}
	return contract.Snapshot{
		Mode:        mode,
		GeneratedAt: snapshot.GeneratedAt,
		Build:       contract.BuildInfo{Version: version, Commit: commit},
		Policy: contract.AttentionPolicy{
			MaxBackupAgeHours:   int(snapshot.Policy.MaxBackupAge / time.Hour),
			StartupGraceMinutes: int(snapshot.Policy.StartupGrace / time.Minute),
		},
		Viewer:        viewer,
		Connections:   mapSlice(snapshot.Connections, connectionContract),
		Deployments:   mapSlice(snapshot.Deployments, deploymentContract),
		Relationships: mapSlice(snapshot.Relationships, relationshipContract),
	}
}

func connectionContract(c fleet.Connection) contract.Connection {
	return contract.Connection{
		Id:            c.ID,
		Name:          c.Name,
		Region:        c.Region,
		Collection:    contract.ConnectionCollection(c.Collection),
		LastAttemptAt: c.LastAttemptAt,
		LastSuccessAt: c.LastSuccessAt,
		Namespaces:    append([]string{}, c.Namespaces...),
		Operators:     mapSlice(c.Operators, operatorContract),
		Capabilities:  mapSlice(c.Capabilities, capabilityContract),
	}
}

func operatorContract(o fleet.Operator) contract.Operator {
	return contract.Operator{
		Id:                 o.ID,
		Namespace:          o.Namespace,
		Name:               o.Name,
		Container:          o.Container,
		Namespaces:         append([]string{}, o.Namespaces...),
		Collection:         contract.OperatorCollection(o.Collection),
		LastAttemptAt:      o.LastAttemptAt,
		LastSuccessAt:      o.LastSuccessAt,
		Health:             contract.OperatorHealth(o.Health),
		Uid:                o.UID,
		Image:              o.Image,
		Version:            o.Version,
		Generation:         o.Generation,
		ObservedGeneration: o.ObservedGeneration,
		DesiredReplicas:    o.DesiredReplicas,
		ReadyReplicas:      o.ReadyReplicas,
		UpdatedReplicas:    o.UpdatedReplicas,
	}
}

func capabilityContract(c fleet.Capability) contract.ApiCapability {
	return contract.ApiCapability{
		Namespace: c.Namespace,
		Resource:  contract.ApiCapabilityResource(c.Resource),
		State:     contract.ApiCapabilityState(c.State),
		CheckedAt: c.CheckedAt,
	}
}

func deploymentContract(d fleet.Deployment) contract.Deployment {
	return contract.Deployment{
		Id:              d.ID,
		Uid:             d.UID,
		Name:            d.Name,
		Namespace:       d.Namespace,
		ConnectionId:    d.ConnectionID,
		Team:            d.Team,
		Environment:     contract.DeploymentEnvironment(d.Environment),
		Location:        d.Location,
		Generation:      d.Generation,
		ResourceVersion: d.ResourceVersion,
		Version:         d.Version,
		DesiredVersion:  d.DesiredVersion,
		ReadyReplicas:   d.ReadyReplicas,
		DesiredReplicas: d.DesiredReplicas,
		Role:            contract.DeploymentRole(d.Role),
		Health:          contract.DeploymentHealth(d.Health),
		OperatorId:      d.OperatorID,
		OperatorVersion: d.OperatorVersion,
		Conditions:      mapSlice(d.Conditions, conditionContract),
		Issues:          mapSlice(d.Issues, issueContract),
		Evidence: contract.Evidence{
			Collection:         contract.EvidenceCollection(d.Evidence.Collection),
			CollectedAt:        d.Evidence.CollectedAt,
			LastAttemptAt:      d.Evidence.LastAttemptAt,
			ProducerFreshness:  contract.EvidenceProducerFreshness(d.Evidence.ProducerFreshness),
			ProducerObservedAt: d.Evidence.ProducerObservedAt,
		},
		UpgradeFailure: optional(d.UpgradeFailure, func(f fleet.OperationFailure) contract.OperationFailure {
			return contract.OperationFailure{Reason: f.Reason, At: f.At}
		}),
		Backup:   optional(d.Backup, backupContract),
		Replica:  optional(d.Replica, replicaContract),
		Recovery: optional(d.Recovery, recoveryContract),
	}
}

func optional[T, R any](value *T, project func(T) R) *R {
	if value == nil {
		return nil
	}
	result := project(*value)
	return &result
}

func conditionContract(c fleet.Condition) contract.Condition {
	return contract.Condition{
		Type:               c.Type,
		Status:             contract.ConditionStatus(c.Status),
		Reason:             c.Reason,
		Message:            c.Message,
		ObservedGeneration: c.ObservedGeneration,
		LastTransitionAt:   c.LastTransitionAt,
	}
}

func issueContract(i fleet.Issue) contract.Issue {
	return contract.Issue{
		Id:            i.ID,
		DeploymentId:  i.DeploymentID,
		Rule:          string(i.Rule),
		Severity:      contract.IssueSeverity(i.Severity),
		Category:      contract.IssueCategory(i.Category),
		Title:         i.Title,
		Detail:        i.Detail,
		FirstSeenAt:   i.FirstSeenAt,
		LastCheckedAt: i.LastCheckedAt,
	}
}

func backupContract(b fleet.Backup) contract.Backup {
	return contract.Backup{
		LastSuccessAt:       b.LastSuccessAt,
		NextAt:              b.NextAt,
		Key:                 b.Key,
		ConsecutiveFailures: b.ConsecutiveFailures,
		Digest:              b.Digest,
		SizeBytes:           b.SizeBytes,
		RestoreTest: optional(b.RestoreTest, func(t fleet.RestoreTest) contract.RestoreTest {
			return contract.RestoreTest{
				Outcome:       contract.RestoreTestOutcome(t.Outcome),
				LastSuccessAt: t.LastSuccessAt,
				FinishedAt:    t.FinishedAt,
				Key:           t.Key,
				Reason:        t.Reason,
				Digest:        t.Digest,
			}
		}),
	}
}

func replicaContract(r fleet.Replica) contract.Replica {
	return contract.Replica{
		ObservedAt:      r.ObservedAt,
		HistoryConflict: r.HistoryConflict,
		LocalIndex:      r.LocalIndex,
		SourceIndex:     r.SourceIndex,
	}
}

func recoveryContract(r fleet.Recovery) contract.Recovery {
	return contract.Recovery{
		RequestName:            r.RequestName,
		Mechanism:              contract.RecoveryMechanism(r.Mechanism),
		Phase:                  contract.RecoveryPhase(r.Phase),
		Stage:                  r.Stage,
		StartedAt:              r.StartedAt,
		CompletedAt:            r.CompletedAt,
		Cleanup:                contract.RecoveryCleanup(r.Cleanup),
		ApplicationConfirmedAt: r.ApplicationConfirmedAt,
		ExclusionAcknowledged:  r.ExclusionAcknowledged,
	}
}

func relationshipContract(r fleet.Relationship) contract.Relationship {
	return contract.Relationship{
		Id:         r.ID,
		SourceId:   r.SourceID,
		TargetId:   r.TargetID,
		Kind:       contract.RelationshipKind(r.Kind),
		Repository: r.Repository,
		Provenance: contract.RelationshipProvenance(r.Provenance),
		ObservedAt: r.ObservedAt,
	}
}

func activityContract(a fleet.Activity) contract.Activity {
	return contract.Activity{
		Id:             a.ID,
		DeploymentId:   a.DeploymentID,
		DeploymentName: a.DeploymentName,
		ConnectionId:   a.ConnectionID,
		Namespace:      a.Namespace,
		RecordedAt:     a.RecordedAt,
		Kind:           contract.ActivityKind(a.Kind),
		Health: optional(a.Health, func(c fleet.HealthChange) contract.HealthChange {
			return contract.HealthChange{From: contract.HealthChangeFrom(c.From), To: contract.HealthChangeTo(c.To)}
		}),
		Collection: optional(a.Collection, func(c fleet.CollectionChange) contract.CollectionChange {
			return contract.CollectionChange{From: contract.CollectionChangeFrom(c.From), To: contract.CollectionChangeTo(c.To)}
		}),
		Finding: optional(a.Finding, func(c fleet.FindingChange) contract.FindingChange {
			return contract.FindingChange{
				Rule:     contract.FindingChangeRule(c.Rule),
				Severity: contract.FindingChangeSeverity(c.Severity),
				PreviousSeverity: optional(c.PreviousSeverity, func(s fleet.Severity) contract.FindingChangePreviousSeverity {
					return contract.FindingChangePreviousSeverity(s)
				}),
			}
		}),
	}
}

func observationContract(o fleet.Observation) contract.Observation {
	return contract.Observation{RecordedAt: o.RecordedAt, Deployment: deploymentContract(o.Deployment)}
}
