// Package demo creates visibly identified, typed demonstration observations.
package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dc-tec/openbao-operator-console/internal/contract"
	"github.com/dc-tec/openbao-operator-console/internal/evidence"
	"github.com/dc-tec/openbao-operator-console/internal/store"
)

type Repository struct {
	snapshot contract.Snapshot
	history  map[string][]contract.Observation
}

func ptr[T any](value T) *T { return &value }

func New(now time.Time) *Repository {
	snapshot := contract.Snapshot{Mode: "demo", CollectedAt: now, Policy: contract.AttentionPolicy{MaxBackupAgeHours: 24, StartupGraceMinutes: 10},
		Deployments: []contract.Deployment{}, Relationships: []contract.Relationship{}, Activity: []contract.Activity{},
		Connections: []contract.Connection{
			{Id: "prod-eu-1", Name: "prod-eu-1", Region: "eu-west-1", Collection: "current", LastAttemptAt: now, LastSuccessAt: &now, Namespaces: []string{"payments", "identity", "platform"}},
			{Id: "prod-eu-2", Name: "prod-eu-2", Region: "eu-central-1", Collection: "current", LastAttemptAt: now, LastSuccessAt: &now, Namespaces: []string{"payments", "customer-x", "finance"}},
			{Id: "dr-eu-1", Name: "dr-eu-1", Region: "eu-central-1", Collection: "current", LastAttemptAt: now, LastSuccessAt: &now, Namespaces: []string{"recovery"}},
			{Id: "nonprod", Name: "nonprod", Region: "eu-west-1", Collection: "current", LastAttemptAt: now, LastSuccessAt: &now, Namespaces: []string{"staging", "development"}},
			{Id: "edge", Name: "edge-dc-1", Region: "on-premises", Collection: "unreachable", LastAttemptAt: now, LastSuccessAt: ptr(now.Add(-24 * time.Hour)), Namespaces: []string{"edge"}},
		},
	}
	for i := range snapshot.Connections {
		connection := &snapshot.Connections[i]
		collection := contract.OperatorCollectionCurrent
		if connection.Collection != "current" {
			collection = "unreachable"
		}
		connection.Operators = []contract.Operator{{Id: "operator-" + connection.Id, Namespace: "operator-system", Name: "openbao-controller", Container: "manager", Namespaces: append([]string{}, connection.Namespaces...), Uid: ptr("demo-controller-" + connection.Id), Image: ptr("example.invalid/openbao-operator:development"), Version: ptr("development"), Collection: collection, LastAttemptAt: now, LastSuccessAt: connection.LastSuccessAt, Health: "available", Generation: 1, ObservedGeneration: ptr(int64(1)), ReadyReplicas: ptr(int32(1)), DesiredReplicas: ptr(int32(1)), UpdatedReplicas: ptr(int32(1))}}
		connection.Capabilities = []contract.ApiCapability{}
		for _, namespace := range connection.Namespaces {
			for _, resource := range []contract.ApiCapabilityResource{"openbaoclusters", "openbaorestores", "openbaorecoveries"} {
				state := contract.ApiCapabilityStateAvailable
				if resource == "openbaorecoveries" {
					state = "unsupported"
				}
				if connection.Collection != "current" {
					state = "unreachable"
				}
				connection.Capabilities = append(connection.Capabilities, contract.ApiCapability{Namespace: namespace, Resource: resource, State: state, CheckedAt: &now})
			}
		}
	}
	rows := []struct {
		id, name, namespace, connection, team, location string
		environment                                     contract.DeploymentEnvironment
		role                                            contract.DeploymentRole
	}{
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
	for _, row := range rows {
		d := contract.Deployment{Id: row.id, Uid: "demo-" + row.id, Name: row.name, Namespace: row.namespace,
			ConnectionId: row.connection, Location: row.location, Team: row.team, Environment: row.environment,
			Version: "2.7.0", DesiredVersion: "2.7.0", OperatorVersion: ptr("development"), Role: row.role,
			Generation: 3, ResourceVersion: "demo-42", ReadyReplicas: ptr(int32(3)), DesiredReplicas: ptr(int32(3)),
			Evidence:   contract.Evidence{CollectedAt: now, LastAttemptAt: now, Collection: "current", ProducerFreshness: "unknown"},
			Conditions: []contract.Condition{{Type: "Available", Status: "True", Reason: "AllReplicasReady", Message: "All configured replicas are ready.", ObservedGeneration: 3, LastTransitionAt: ptr(now.Add(-48 * time.Hour))}},
			Backup: &contract.Backup{LastSuccessAt: ptr(now.Add(-14 * time.Minute)), NextAt: ptr(now.Add(24 * time.Hour)), Key: ptr(row.namespace + "/" + now.Format("2006-01-02") + ".snapshot"), Digest: ptr("sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), SizeBytes: ptr(int64(1288490188)),
				RestoreTest: &contract.RestoreTest{Outcome: "passed", FinishedAt: ptr(now.Add(-72 * time.Hour)), LastSuccessAt: ptr(now.Add(-72 * time.Hour)), Key: ptr(row.namespace + "/restore-test.snapshot"), Reason: "SnapshotApplied"}},
		}
		if row.role == "replica" {
			d.Backup = nil
			d.Replica = &contract.Replica{ObservedAt: ptr(now.Add(-12 * time.Second)), LocalIndex: ptr("9007199254741007"), SourceIndex: ptr("9007199254741014")}
		}
		d.OperatorId = ptr("operator-" + row.connection)
		if row.role == "recoveryTarget" {
			d.Backup = nil
			d.ReadyReplicas = ptr(int32(1))
			d.DesiredReplicas = ptr(int32(1))
			d.Recovery = &contract.Recovery{RequestName: "payments-recovery-drill", Mechanism: "snapshot", Phase: "completed", Stage: "FollowThroughComplete", StartedAt: ptr(now.Add(-73 * time.Hour)), CompletedAt: ptr(now.Add(-72 * time.Hour)), ApplicationConfirmedAt: ptr(now.Add(-72 * time.Hour)), Cleanup: "notApplicable"}
		}
		if row.id == "identity" {
			d.ReadyReplicas = ptr(int32(0))
			d.Conditions = []contract.Condition{{Type: "OpenBaoSealed", Status: "True", Reason: "Sealed", Message: "The operator reports sealed state.", ObservedGeneration: 3, LastTransitionAt: ptr(now.Add(-3 * time.Hour))}}
		}
		if row.id == "customer-x" {
			d.Backup.ConsecutiveFailures = 2
			d.Backup.LastSuccessAt = ptr(now.Add(-19 * time.Hour))
		}
		if row.id == "staging" {
			d.Version = "2.6.1"
			d.UpgradeFailure = &contract.OperationFailure{Reason: "PodNotReady", At: ptr(now.Add(-7 * time.Hour))}
			d.Conditions = append(d.Conditions, contract.Condition{Type: "Upgrading", Status: "False", Reason: "UpgradeFailed", Message: "Post-upgrade checks failed: PodNotReady.", ObservedGeneration: 3, LastTransitionAt: ptr(now.Add(-7 * time.Hour))})
		}
		if row.id == "edge" {
			d.Backup = nil
			d.ReadyReplicas = nil
			d.Evidence.CollectedAt = now.Add(-24 * time.Hour)
		}
		evidence.Evaluate(&d, now.Add(-7*24*time.Hour), false, evidence.Policy{MaxBackupAge: 24 * time.Hour, StartupGrace: 10 * time.Minute}, now)
		if row.id == "edge" {
			evidence.CollectionFailure(&d, "unreachable", now)
		}
		for i := range d.Issues {
			d.Issues[i].FirstSeenAt = now.Add(-3 * time.Hour)
		}
		snapshot.Deployments = append(snapshot.Deployments, d)
	}
	snapshot.Relationships = []contract.Relationship{
		{Id: "payments-replication", SourceId: "payments", TargetId: "payments-read", Kind: "replication", Provenance: "declared", ObservedAt: ptr(now.Add(-12 * time.Second))},
		{Id: "payments-recovery", SourceId: "payments", TargetId: "payments-dr", Kind: "snapshotRecovery", Provenance: "declared", Repository: ptr("s3://platform-recovery/payments")},
	}
	snapshot.Activity = []contract.Activity{
		{Id: "event-1", DeploymentId: "payments", At: now.Add(-14 * time.Minute), Title: "Backup completed", Detail: "Successful Job completion recorded · payments-bao", Outcome: "success"},
		{Id: "event-2", DeploymentId: "customer-x", At: now.Add(-2 * time.Hour), Title: "Backup failure observed", Detail: "Two consecutive failures · customer-x-bao", Outcome: "warning"},
		{Id: "event-3", DeploymentId: "identity", At: now.Add(-3 * time.Hour), Title: "Sealed state observed", Detail: "No ready replicas · identity-bao", Outcome: "error"},
		{Id: "event-4", DeploymentId: "staging", At: now.Add(-7 * time.Hour), Title: "Upgrade failure observed", Detail: "Post-upgrade checks · staging-bao", Outcome: "warning"},
		{Id: "event-5", DeploymentId: "payments-dr", At: now.Add(-72 * time.Hour), Title: "Recovery target application confirmed", Detail: "Expected source identity · historical receipt", Outcome: "success"},
	}
	repository := &Repository{snapshot: snapshot, history: map[string][]contract.Observation{}}
	for _, deployment := range snapshot.Deployments {
		repository.history[deployment.Id] = history(deployment, now)
	}
	return repository
}

// copyDeployment returns an independent deep copy, so history entries never share pointers.
func copyDeployment(d contract.Deployment) contract.Deployment {
	encoded, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var result contract.Deployment
	if err := json.Unmarshal(encoded, &result); err != nil {
		panic(err)
	}
	return result
}

type demoEvent struct {
	at    time.Time
	apply func(*contract.Deployment)
}

// history synthesizes a week of fictional observations that ends in the current fixture state.
// Like the collector, it records an observation only for an event that changes the deployment.
func history(current contract.Deployment, now time.Time) []contract.Observation {
	policy := evidence.Policy{MaxBackupAge: 24 * time.Hour, StartupGrace: 10 * time.Minute}
	created := now.Add(-7 * 24 * time.Hour)
	start := created.Add(time.Hour)
	state := copyDeployment(current)
	if state.DesiredReplicas != nil {
		state.ReadyReplicas = ptr(*state.DesiredReplicas)
	}
	state.UpgradeFailure = nil
	state.Recovery = nil
	state.Conditions = []contract.Condition{{Type: "Available", Status: "True", Reason: "AllReplicasReady", Message: "All configured replicas are ready.", ObservedGeneration: state.Generation, LastTransitionAt: ptr(created)}}
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
			events = append(events, demoEvent{at, func(d *contract.Deployment) {
				d.Backup.LastSuccessAt, d.Backup.SizeBytes, d.Backup.Key, d.Backup.ConsecutiveFailures = ptr(at), ptr(size), ptr(key), 0
			}})
		}
		if test := backup.RestoreTest; test != nil && test.FinishedAt != nil {
			events = append(events, demoEvent{*test.FinishedAt, func(d *contract.Deployment) {
				d.Backup.RestoreTest = &contract.RestoreTest{Outcome: test.Outcome, FinishedAt: test.FinishedAt, LastSuccessAt: test.LastSuccessAt, Key: test.Key, Reason: test.Reason}
			}})
		}
	}
	switch current.Id {
	case "identity":
		events = append(events,
			demoEvent{now.Add(-3*time.Hour - 4*time.Minute), func(d *contract.Deployment) { d.ReadyReplicas = ptr(int32(1)) }},
			demoEvent{now.Add(-3 * time.Hour), func(d *contract.Deployment) {
				d.ReadyReplicas, d.Conditions = current.ReadyReplicas, current.Conditions
			}})
	case "customer-x":
		events = append(events, demoEvent{now.Add(-8 * time.Hour), func(d *contract.Deployment) { d.Backup.ConsecutiveFailures = 1 }})
		later(now.Add(-2 * time.Hour))
	case "staging":
		state.DesiredVersion, state.Version, state.Generation = "2.6.1", "2.6.1", 2
		state.Conditions[0].ObservedGeneration = 2
		events = append(events,
			demoEvent{now.Add(-8 * time.Hour), func(d *contract.Deployment) {
				d.DesiredVersion, d.Generation = "2.7.0", 3
				d.Conditions = []contract.Condition{d.Conditions[0], {Type: "Upgrading", Status: "True", Reason: "RollingUpgrade", Message: "Upgrading from 2.6.1 to 2.7.0.", ObservedGeneration: 3, LastTransitionAt: ptr(now.Add(-8 * time.Hour))}}
				d.Conditions[0].ObservedGeneration = 3
			}},
			demoEvent{now.Add(-7 * time.Hour), func(d *contract.Deployment) {
				d.UpgradeFailure, d.Conditions = current.UpgradeFailure, current.Conditions
			}})
	case "edge":
		later(now.Add(-24*time.Hour + 2*time.Minute))
	case "payments-dr":
		events = append(events, demoEvent{now.Add(-73 * time.Hour), func(d *contract.Deployment) {
			recovery := *current.Recovery
			recovery.Phase, recovery.Stage, recovery.CompletedAt, recovery.ApplicationConfirmedAt = "running", "SnapshotApplying", nil, nil
			d.Recovery = &recovery
		}})
		later(now.Add(-72 * time.Hour))
	}
	observations := []contract.Observation{}
	record := func(at time.Time, d contract.Deployment) {
		d.ResourceVersion = fmt.Sprintf("demo-%d", len(observations)+1)
		d.Evidence.CollectedAt, d.Evidence.LastAttemptAt, d.Evidence.Collection = at, at, "current"
		evidence.Evaluate(&d, created, false, policy, at)
		observations = append(observations, contract.Observation{RecordedAt: at, Deployment: d})
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
	observations = append(observations, contract.Observation{RecordedAt: final, Deployment: last})
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

func (r *Repository) Snapshot(context.Context) (contract.Snapshot, error) { return r.snapshot, nil }
func (r *Repository) Apply(context.Context, store.Batch) error            { return nil }
func (r *Repository) Close()                                              {}
func (r *Repository) History(_ context.Context, id string) ([]contract.Observation, error) {
	if observations, ok := r.history[id]; ok {
		return observations, nil
	}
	return []contract.Observation{}, nil
}
