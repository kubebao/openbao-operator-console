// Package demo creates visibly identified, typed demonstration observations.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kubebao/openbao-operator-console/internal/config"
	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

type Repository struct {
	snapshot fleet.Snapshot
	history  map[string][]fleet.Observation
}

func ptr[T any](value T) *T { return &value }

// policy matches the default enrollment thresholds.
var policy = fleet.Policy{MaxBackupAge: 24 * time.Hour, StartupGrace: 10 * time.Minute}

func New(now time.Time) *Repository {
	snapshot := fleet.Snapshot{
		CollectedAt:   now,
		Policy:        policy,
		Connections:   connections(now),
		Deployments:   []fleet.Deployment{},
		Relationships: relationships(now),
		Activity:      activity(now),
	}
	for _, row := range fixtures {
		snapshot.Deployments = append(snapshot.Deployments, deployment(row, now))
	}
	repository := &Repository{snapshot: snapshot, history: map[string][]fleet.Observation{}}
	for _, deployment := range snapshot.Deployments {
		repository.history[deployment.ID] = history(deployment, now)
	}
	return repository
}

// connections are the fixture clusters. The edge cluster is unreachable.
func connections(now time.Time) []fleet.Connection {
	result := []fleet.Connection{
		{
			ID:            "prod-eu-1",
			Name:          "prod-eu-1",
			Region:        "eu-west-1",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: now,
			LastSuccessAt: &now,
			Namespaces:    []string{"payments", "identity", "platform"},
		},
		{
			ID:            "prod-eu-2",
			Name:          "prod-eu-2",
			Region:        "eu-central-1",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: now,
			LastSuccessAt: &now,
			Namespaces:    []string{"payments", "customer-x", "finance"},
		},
		{
			ID:            "dr-eu-1",
			Name:          "dr-eu-1",
			Region:        "eu-central-1",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: now,
			LastSuccessAt: &now,
			Namespaces:    []string{"recovery"},
		},
		{
			ID:            "nonprod",
			Name:          "nonprod",
			Region:        "eu-west-1",
			Collection:    fleet.ConnectionCollectionCurrent,
			LastAttemptAt: now,
			LastSuccessAt: &now,
			Namespaces:    []string{"staging", "development"},
		},
		{
			ID:            "edge",
			Name:          "edge-dc-1",
			Region:        "on-premises",
			Collection:    fleet.ConnectionCollectionUnreachable,
			LastAttemptAt: now,
			LastSuccessAt: ptr(now.Add(-24 * time.Hour)),
			Namespaces:    []string{"edge"},
		},
	}
	for i := range result {
		connection := &result[i]
		collection := fleet.OperatorCollectionCurrent
		if connection.Collection != fleet.ConnectionCollectionCurrent {
			collection = fleet.OperatorCollectionUnreachable
		}
		connection.Operators = []fleet.Operator{
			{
				ID:                 "operator-" + connection.ID,
				Namespace:          "operator-system",
				Name:               "openbao-controller",
				Container:          "manager",
				Namespaces:         append([]string{}, connection.Namespaces...),
				UID:                ptr("demo-controller-" + connection.ID),
				Image:              ptr("example.invalid/openbao-operator:development"),
				Version:            ptr("development"),
				Collection:         collection,
				LastAttemptAt:      now,
				LastSuccessAt:      connection.LastSuccessAt,
				Health:             fleet.OperatorHealthAvailable,
				Generation:         1,
				ObservedGeneration: ptr(int64(1)),
				ReadyReplicas:      ptr(int32(1)),
				DesiredReplicas:    ptr(int32(1)),
				UpdatedReplicas:    ptr(int32(1)),
			},
		}
		connection.Capabilities = []fleet.Capability{}
		for _, namespace := range connection.Namespaces {
			for _, resource := range []fleet.CapabilityResource{
				fleet.CapabilityResourceClusters,
				fleet.CapabilityResourceRestores,
				fleet.CapabilityResourceRecoveries,
			} {
				state := fleet.CapabilityStateAvailable
				if resource == fleet.CapabilityResourceRecoveries {
					state = fleet.CapabilityStateUnsupported
				}
				if connection.Collection != fleet.ConnectionCollectionCurrent {
					state = fleet.CapabilityStateUnreachable
				}
				connection.Capabilities = append(
					connection.Capabilities,
					fleet.Capability{Namespace: namespace, Resource: resource, State: state, CheckedAt: &now},
				)
			}
		}
	}
	return result
}

// fixture describes one fictional deployment.
type fixture struct {
	id, name, namespace, connection, team, location string
	environment                                     fleet.Environment
	role                                            fleet.Role
}

var fixtures = []fixture{
	{"payments", "payments-bao", "payments", "prod-eu-1", "Payments", "Amsterdam", "production", "primary"},
	{"payments-read", "payments-read", "payments", "prod-eu-2", "Payments", "Frankfurt", "production", "replica"},
	{"payments-dr", "payments-dr", "recovery", "dr-eu-1", "Payments", "Frankfurt", "production", "recoveryTarget"},
	{"identity", "identity-bao", "identity", "prod-eu-1", "Identity", "Amsterdam", "production", "primary"},
	{"platform", "platform-bao", "platform", "prod-eu-1", "Platform", "Amsterdam", "production", "primary"},
	{"customer-x", "customer-x-bao", "customer-x", "prod-eu-2", "Customer X", "Frankfurt", "production", "primary"},
	{"finance", "finance-bao", "finance", "prod-eu-2", "Finance", "Frankfurt", "production", "primary"},
	{"staging", "staging-bao", "staging", "nonprod", "Platform", "Amsterdam", "staging", "primary"},
	{"dev", "dev-bao", "development", "nonprod", "Platform", "Amsterdam", "development", "primary"},
	{"sandbox", "sandbox-bao", "development", "nonprod", "Platform", "Amsterdam", "development", "primary"},
	{"internal", "internal-bao", "platform", "prod-eu-1", "Security", "Amsterdam", "production", "primary"},
	{"edge", "edge-bao", "edge", "edge", "Edge", "Rotterdam", "other", "primary"},
}

// deployment builds a fixture's current state. Findings use the same rules as live
// collection; their first-seen times are fixed for a stable demonstration.
func deployment(row fixture, now time.Time) fleet.Deployment {
	d := fleet.Deployment{
		ID:              row.id,
		UID:             "demo-" + row.id,
		Name:            row.name,
		Namespace:       row.namespace,
		ConnectionID:    row.connection,
		Location:        row.location,
		Team:            row.team,
		Environment:     row.environment,
		Version:         "2.7.0",
		DesiredVersion:  "2.7.0",
		OperatorVersion: ptr("development"),
		Role:            row.role,
		Generation:      3,
		ResourceVersion: "demo-42",
		ReadyReplicas:   ptr(int32(3)),
		DesiredReplicas: ptr(int32(3)),
		Evidence: fleet.Evidence{
			CollectedAt:       now,
			LastAttemptAt:     now,
			Collection:        fleet.EvidenceCollectionCurrent,
			ProducerFreshness: fleet.FreshnessUnknown,
		},
		Conditions: []fleet.Condition{
			{
				Type:               "Available",
				Status:             fleet.ConditionStatusTrue,
				Reason:             "AllReplicasReady",
				Message:            "All configured replicas are ready.",
				ObservedGeneration: 3,
				LastTransitionAt:   ptr(now.Add(-48 * time.Hour)),
			},
		},
		Backup: &fleet.Backup{
			LastSuccessAt: ptr(
				now.Add(-14 * time.Minute),
			),
			NextAt:    ptr(now.Add(24 * time.Hour)),
			Key:       ptr(row.namespace + "/" + now.Format("2006-01-02") + ".snapshot"),
			Digest:    ptr("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
			SizeBytes: ptr(int64(1288490188)),
			RestoreTest: &fleet.RestoreTest{
				Outcome:       fleet.RestoreOutcomePassed,
				FinishedAt:    ptr(now.Add(-72 * time.Hour)),
				LastSuccessAt: ptr(now.Add(-72 * time.Hour)),
				Key:           ptr(row.namespace + "/restore-test.snapshot"),
				Reason:        "SnapshotApplied",
			},
		},
	}
	if row.role == fleet.RoleReplica {
		d.Backup = nil
		d.Replica = &fleet.Replica{
			ObservedAt:  ptr(now.Add(-12 * time.Second)),
			LocalIndex:  ptr("9007199254741007"),
			SourceIndex: ptr("9007199254741014"),
		}
	}
	d.OperatorID = ptr("operator-" + row.connection)
	if row.role == fleet.RoleRecoveryTarget {
		d.Backup = nil
		d.ReadyReplicas = ptr(int32(1))
		d.DesiredReplicas = ptr(int32(1))
		d.Recovery = &fleet.Recovery{
			RequestName:            "payments-recovery-drill",
			Mechanism:              fleet.RecoveryMechanismSnapshot,
			Phase:                  fleet.RecoveryPhaseCompleted,
			Stage:                  "FollowThroughComplete",
			StartedAt:              ptr(now.Add(-73 * time.Hour)),
			CompletedAt:            ptr(now.Add(-72 * time.Hour)),
			ApplicationConfirmedAt: ptr(now.Add(-72 * time.Hour)),
			Cleanup:                fleet.RecoveryCleanupNotApplicable,
		}
	}
	if row.id == "identity" {
		d.ReadyReplicas = ptr(int32(0))
		d.Conditions = []fleet.Condition{
			{
				Type:               "OpenBaoSealed",
				Status:             fleet.ConditionStatusTrue,
				Reason:             "Sealed",
				Message:            "The operator reports sealed state.",
				ObservedGeneration: 3,
				LastTransitionAt:   ptr(now.Add(-3 * time.Hour)),
			},
		}
	}
	if row.id == "customer-x" {
		d.Backup.ConsecutiveFailures = 2
		d.Backup.LastSuccessAt = ptr(now.Add(-19 * time.Hour))
	}
	if row.id == "staging" {
		d.Version = "2.6.1"
		d.UpgradeFailure = &fleet.OperationFailure{Reason: "PodNotReady", At: ptr(now.Add(-7 * time.Hour))}
		d.Conditions = append(
			d.Conditions,
			fleet.Condition{
				Type:               "Upgrading",
				Status:             fleet.ConditionStatusFalse,
				Reason:             "UpgradeFailed",
				Message:            "Post-upgrade checks failed: PodNotReady.",
				ObservedGeneration: 3,
				LastTransitionAt:   ptr(now.Add(-7 * time.Hour)),
			},
		)
	}
	if row.id == "edge" {
		d.Backup = nil
		d.ReadyReplicas = nil
		d.Evidence.CollectedAt = now.Add(-24 * time.Hour)
	}
	evidence.Evaluate(
		&d,
		now.Add(-7*24*time.Hour),
		false,
		policy,
		now,
	)
	if row.id == "edge" {
		evidence.CollectionFailure(&d, fleet.EvidenceCollectionUnreachable, now)
	}
	for i := range d.Issues {
		d.Issues[i].FirstSeenAt = now.Add(-3 * time.Hour)
	}
	return d
}

func relationships(now time.Time) []fleet.Relationship {
	return []fleet.Relationship{
		{
			ID:         "payments-replication",
			SourceID:   "payments",
			TargetID:   "payments-read",
			Kind:       fleet.RelationshipKindReplication,
			Provenance: fleet.ProvenanceDeclared,
			ObservedAt: ptr(now.Add(-12 * time.Second)),
		},
		{
			ID:         "payments-recovery",
			SourceID:   "payments",
			TargetID:   "payments-dr",
			Kind:       fleet.RelationshipKindSnapshotRecovery,
			Provenance: fleet.ProvenanceDeclared,
			Repository: ptr("s3://platform-recovery/payments"),
		},
	}
}

func activity(now time.Time) []fleet.Activity {
	return []fleet.Activity{
		{
			ID:           "event-1",
			DeploymentID: "payments",
			At:           now.Add(-14 * time.Minute),
			Title:        "Backup completed",
			Detail:       "Successful Job completion recorded · payments-bao",
			Outcome:      fleet.ActivityOutcomeSuccess,
		},
		{
			ID:           "event-2",
			DeploymentID: "customer-x",
			At:           now.Add(-2 * time.Hour),
			Title:        "Backup failure observed",
			Detail:       "Two consecutive failures · customer-x-bao",
			Outcome:      fleet.ActivityOutcomeWarning,
		},
		{
			ID:           "event-3",
			DeploymentID: "identity",
			At:           now.Add(-3 * time.Hour),
			Title:        "Sealed state observed",
			Detail:       "No ready replicas · identity-bao",
			Outcome:      fleet.ActivityOutcomeError,
		},
		{
			ID:           "event-4",
			DeploymentID: "staging",
			At:           now.Add(-7 * time.Hour),
			Title:        "Upgrade failure observed",
			Detail:       "Post-upgrade checks · staging-bao",
			Outcome:      fleet.ActivityOutcomeWarning,
		},
		{
			ID:           "event-5",
			DeploymentID: "payments-dr",
			At:           now.Add(-72 * time.Hour),
			Title:        "Recovery target application confirmed",
			Detail:       "Expected source identity · historical receipt",
			Outcome:      fleet.ActivityOutcomeSuccess,
		},
	}
}

// copyDeployment returns an independent deep copy, so history entries never share pointers.
func copyDeployment(d fleet.Deployment) fleet.Deployment {
	encoded, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var result fleet.Deployment
	if err := json.Unmarshal(encoded, &result); err != nil {
		panic(err)
	}
	return result
}

type demoEvent struct {
	at    time.Time
	apply func(*fleet.Deployment)
}

// history synthesizes a week of fictional observations that ends in the current fixture state.
// Like the collector, it records an observation only for an event that changes the deployment.
func history(current fleet.Deployment, now time.Time) []fleet.Observation {
	created := now.Add(-7 * 24 * time.Hour)
	start := created.Add(time.Hour)
	state := copyDeployment(current)
	if state.DesiredReplicas != nil {
		state.ReadyReplicas = ptr(*state.DesiredReplicas)
	}
	state.UpgradeFailure = nil
	state.Recovery = nil
	state.Conditions = []fleet.Condition{
		{
			Type:               "Available",
			Status:             fleet.ConditionStatusTrue,
			Reason:             "AllReplicasReady",
			Message:            "All configured replicas are ready.",
			ObservedGeneration: state.Generation,
			LastTransitionAt:   ptr(created),
		},
	}
	events := []demoEvent{}
	// The final observation is the current fixture, recorded at the latest change.
	final := start
	later := func(at time.Time) {
		if at.After(final) {
			final = at
		}
	}
	if backup := current.Backup; backup != nil && backup.LastSuccessAt != nil {
		last := *backup.LastSuccessAt
		later(last)
		state.Backup.ConsecutiveFailures = 0
		if test := state.Backup.RestoreTest; test != nil {
			test.FinishedAt, test.LastSuccessAt = ptr(now.Add(-9*24*time.Hour)), ptr(now.Add(-9*24*time.Hour))
		}
		for day := 6; day >= 1; day-- {
			at := last.Add(-time.Duration(day) * 24 * time.Hour)
			size := int64(float64(*backup.SizeBytes) * (1 - 0.012*float64(day)))
			key := fmt.Sprintf("%s/%s.snapshot", current.Namespace, at.Format("2006-01-02"))
			events = append(events, demoEvent{at, func(d *fleet.Deployment) {
				d.Backup.LastSuccessAt, d.Backup.SizeBytes, d.Backup.Key, d.Backup.ConsecutiveFailures = ptr(at), ptr(size), ptr(key), 0
			}})
		}
		if test := backup.RestoreTest; test != nil && test.FinishedAt != nil {
			events = append(events, demoEvent{*test.FinishedAt, func(d *fleet.Deployment) {
				d.Backup.RestoreTest = &fleet.RestoreTest{
					Outcome:       test.Outcome,
					FinishedAt:    test.FinishedAt,
					LastSuccessAt: test.LastSuccessAt,
					Key:           test.Key,
					Reason:        test.Reason,
				}
			}})
		}
	}
	switch current.ID {
	case "identity":
		events = append(events,
			demoEvent{now.Add(-3*time.Hour - 4*time.Minute), func(d *fleet.Deployment) { d.ReadyReplicas = ptr(int32(1)) }},
			demoEvent{now.Add(-3 * time.Hour), func(d *fleet.Deployment) {
				d.ReadyReplicas, d.Conditions = current.ReadyReplicas, current.Conditions
			}})
	case "customer-x":
		events = append(events, demoEvent{now.Add(-8 * time.Hour), func(d *fleet.Deployment) { d.Backup.ConsecutiveFailures = 1 }})
		later(now.Add(-2 * time.Hour))
	case "staging":
		state.DesiredVersion, state.Version, state.Generation = "2.6.1", "2.6.1", 2
		state.Conditions[0].ObservedGeneration = 2
		events = append(events,
			demoEvent{now.Add(-8 * time.Hour), func(d *fleet.Deployment) {
				d.DesiredVersion, d.Generation = "2.7.0", 3
				d.Conditions = []fleet.Condition{
					d.Conditions[0],
					{
						Type:               "Upgrading",
						Status:             fleet.ConditionStatusTrue,
						Reason:             "RollingUpgrade",
						Message:            "Upgrading from 2.6.1 to 2.7.0.",
						ObservedGeneration: 3,
						LastTransitionAt:   ptr(now.Add(-8 * time.Hour)),
					},
				}
				d.Conditions[0].ObservedGeneration = 3
			}},
			demoEvent{now.Add(-7 * time.Hour), func(d *fleet.Deployment) {
				d.UpgradeFailure, d.Conditions = current.UpgradeFailure, current.Conditions
			}})
	case "edge":
		later(now.Add(-24*time.Hour + 2*time.Minute))
	case "payments-dr":
		events = append(events, demoEvent{now.Add(-73 * time.Hour), func(d *fleet.Deployment) {
			recovery := *current.Recovery
			recovery.Phase, recovery.Stage, recovery.CompletedAt = fleet.RecoveryPhaseRunning, "SnapshotApplying", nil
			recovery.ApplicationConfirmedAt = nil
			d.Recovery = &recovery
		}})
		later(now.Add(-72 * time.Hour))
	}
	observations := []fleet.Observation{}
	record := func(at time.Time, d fleet.Deployment) {
		d.ResourceVersion = fmt.Sprintf("demo-%d", len(observations)+1)
		d.Evidence.CollectedAt, d.Evidence.LastAttemptAt, d.Evidence.Collection = at, at, fleet.EvidenceCollectionCurrent
		evidence.Evaluate(&d, created, false, policy, at)
		observations = append(observations, fleet.Observation{RecordedAt: at, Deployment: d})
	}
	if final.After(start) {
		record(start, copyDeployment(state))
	}
	for _, event := range sortEvents(events) {
		if event.at.After(start) && event.at.Before(final) {
			event.apply(&state)
			record(event.at, copyDeployment(state))
		}
	}
	last := copyDeployment(current)
	last.ResourceVersion = fmt.Sprintf("demo-%d", len(observations)+1)
	observations = append(observations, fleet.Observation{RecordedAt: final, Deployment: last})
	// The API returns the newest observation first.
	for i, j := 0, len(observations)-1; i < j; i, j = i+1, j-1 {
		observations[i], observations[j] = observations[j], observations[i]
	}
	return observations
}

func sortEvents(events []demoEvent) []demoEvent {
	sorted := append([]demoEvent{}, events...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].at.Before(sorted[j-1].at); j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return sorted
}

func (r *Repository) Snapshot(context.Context) (fleet.Snapshot, error) { return r.snapshot, nil }

// Viewer is the demonstration viewer. Its scopes cover every fixture namespace, so demo
// mode uses the same authorization path as live mode.
func (r *Repository) Viewer() config.User {
	viewer := config.User{Subject: "local-demo", Name: "Platform team", Scopes: []config.Scope{}}
	for _, connection := range r.snapshot.Connections {
		viewer.Scopes = append(
			viewer.Scopes,
			config.Scope{ConnectionID: connection.ID, Namespaces: append([]string{}, connection.Namespaces...)},
		)
	}
	return viewer
}

func (r *Repository) History(_ context.Context, id string) ([]fleet.Observation, error) {
	if observations, ok := r.history[id]; ok {
		return observations, nil
	}
	return []fleet.Observation{}, nil
}
