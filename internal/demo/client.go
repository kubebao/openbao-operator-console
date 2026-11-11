package demo

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/kubebao/openbao-operator-client/adapter"
	"github.com/kubebao/openbao-operator-client/diagnose"
	shared "github.com/kubebao/openbao-operator-client/evidence"

	"github.com/kubebao/openbao-operator-console/internal/evidence"
	"github.com/kubebao/openbao-operator-console/internal/fleet"
)

// evaluateFixture creates public-API fixtures from controlled demonstration data.
// It is not a reconstruction path for persisted or live observations.
func evaluateFixture(d *fleet.Deployment, created time.Time, paused bool, policy fleet.Policy, at time.Time) {
	data, err := json.Marshal(fixtureCluster(d, created, paused))
	if err != nil {
		panic("invalid demonstration cluster")
	}
	options := adapter.ProjectionOptions{
		Origin:      d.ConnectionID,
		Contract:    adapter.V1Alpha1Snapshot,
		CollectedAt: at,
		Limits:      adapter.DefaultProjectionLimits(),
	}
	cluster, err := adapter.ProjectCluster(data, options)
	if err != nil {
		panic("invalid demonstration projection")
	}
	read := shared.Collection{
		Scope: shared.Scope{
			Origin:    d.ConnectionID,
			Group:     "openbao.org",
			Version:   "v1alpha1",
			Resource:  "openbaoclusters",
			Namespace: d.Namespace,
			Action:    shared.ReadList,
		},
		Interval: shared.Interval{StartedAt: at, CompletedAt: at},
		Outcome:  shared.CollectionComplete,
		Items:    1,
	}
	verification, err := shared.NewTargetVerification(cluster, read, []shared.Cluster{cluster})
	if err != nil {
		panic("invalid demonstration verification")
	}
	requests := []shared.Restore{}
	if recovery := d.Recovery; recovery != nil && recovery.Mechanism == fleet.RecoveryMechanismSnapshot {
		phase := string(recovery.Phase)
		if phase != "" {
			phase = strings.ToUpper(phase[:1]) + phase[1:]
		}
		stage := "Created"
		if recovery.Phase == fleet.RecoveryPhaseCompleted {
			stage = "FollowThroughComplete"
		}
		if recovery.Phase == fleet.RecoveryPhaseUnknown {
			stage = "Unknown"
		}
		request := map[string]any{
			"apiVersion": "openbao.org/v1alpha1",
			"kind":       "OpenBaoRestore",
			"metadata": map[string]any{
				"name":              recovery.RequestName,
				"namespace":         d.Namespace,
				"uid":               "fixture-restore-" + d.UID,
				"creationTimestamp": created.Format(time.RFC3339Nano),
			},
			"spec": map[string]any{
				"cluster": d.Name,
			},
			"status": map[string]any{
				"phase":     phase,
				"target":    map[string]any{"uid": d.UID},
				"execution": map[string]any{"targetUID": d.UID, "stage": stage},
			},
		}
		raw, marshalErr := json.Marshal(request)
		if marshalErr != nil {
			panic("invalid demonstration restore")
		}
		restore, projectErr := adapter.ProjectRestore(raw, options)
		if projectErr != nil {
			panic("invalid demonstration restore projection")
		}
		requests = append(requests, restore)
	}
	restoreRead := read
	restoreRead.Scope.Resource = "openbaorestores"
	restoreRead.Items = len(requests)
	input := diagnose.ClusterInput{
		Subject:         cluster.Reference.Identity,
		AdapterContract: cluster.AdapterContract,
		Observation:     &cluster,
		Collection:      read,
		Verification:    &verification,
		Restores: &diagnose.ResourceInput[shared.Restore]{
			AdapterContract: cluster.AdapterContract,
			Collection:      restoreRead,
			Observations:    requests,
		},
	}
	p := diagnose.DefaultPolicy(at, []string{cluster.AdapterContract})
	p.Rules = evidence.Rules()
	p.MaxBackupAge, p.StartupGrace = policy.MaxBackupAge, policy.StartupGrace
	p.RestoreUIDBindingContracts = []string{cluster.AdapterContract}
	if d.Backup == nil {
		selected := p.Rules[:0]
		for _, rule := range p.Rules {
			if rule != diagnose.RuleRestoreTest {
				selected = append(selected, rule)
			}
		}
		p.Rules = selected
	}
	report, err := diagnose.Analyze(diagnose.Capture{SchemaVersion: shared.SchemaVersion, Clusters: []diagnose.ClusterInput{input}}, p)
	if err != nil {
		panic("invalid demonstration analysis")
	}
	evidence.Apply(d, report)
}

func fixtureCluster(d *fleet.Deployment, created time.Time, paused bool) map[string]any {
	generation := d.Generation
	if generation <= 0 {
		generation = 1
	}
	metadata := map[string]any{
		"name":              d.Name,
		"namespace":         d.Namespace,
		"uid":               d.UID,
		"generation":        generation,
		"creationTimestamp": created.Format(time.RFC3339Nano),
	}
	spec := map[string]any{"paused": paused}
	status := map[string]any{}
	if d.ReadyReplicas != nil {
		status["observedGeneration"] = generation
	}
	if d.DesiredReplicas != nil {
		spec["replicas"] = *d.DesiredReplicas
	}
	if d.ReadyReplicas != nil {
		status["readyReplicas"] = *d.ReadyReplicas
	}
	conditions := []any{}
	sealed := false
	for _, c := range d.Conditions {
		value := map[string]any{"type": c.Type, "status": string(c.Status), "observedGeneration": c.ObservedGeneration}
		if c.Reason != "" {
			value["reason"] = c.Reason
		}
		if c.LastTransitionAt != nil {
			value["lastTransitionTime"] = c.LastTransitionAt.Format(time.RFC3339Nano)
		}
		conditions = append(conditions, value)
		sealed = sealed || c.Type == "OpenBaoSealed"
	}
	if !sealed {
		conditions = append(conditions, map[string]any{"type": "OpenBaoSealed", "status": "False", "observedGeneration": generation})
	}
	status["conditions"] = conditions
	if d.UpgradeFailure != nil {
		failure := map[string]any{"reason": d.UpgradeFailure.Reason}
		if d.UpgradeFailure.At != nil {
			failure["at"] = d.UpgradeFailure.At.Format(time.RFC3339Nano)
		}
		status["upgrade"] = map[string]any{"failure": failure}
	}
	fixtureBackup(d.Backup, generation, spec, status)
	return map[string]any{
		"apiVersion": "openbao.org/v1alpha1",
		"kind":       "OpenBaoCluster",
		"metadata":   metadata,
		"spec":       spec,
		"status":     status,
	}
}

func fixtureBackup(b *fleet.Backup, generation int64, spec, status map[string]any) {
	if b != nil {
		spec["backup"] = map[string]any{}
		backup := map[string]any{}
		if b.ConsecutiveFailures != nil {
			backup["consecutiveFailures"] = *b.ConsecutiveFailures
		}
		if b.LastSuccessAt != nil {
			backup["lastBackupTime"] = b.LastSuccessAt.Format(time.RFC3339Nano)
		}
		if test := b.RestoreTest; test != nil {
			value := map[string]any{}
			check := map[string]any{"type": "Passed", "status": "True", "reason": "Passed", "observedGeneration": generation}
			switch test.Outcome {
			case fleet.RestoreOutcomeRunning:
				value["active"] = map[string]any{}
				check["status"], check["reason"] = "Unknown", "Running"
			case fleet.RestoreOutcomeBlocked:
				check["status"], check["reason"] = "False", "Blocked"
			case fleet.RestoreOutcomeFailed, fleet.RestoreOutcomePassed:
				outcome := "Passed"
				if test.Outcome == fleet.RestoreOutcomeFailed {
					outcome = "Failed"
					check["status"], check["reason"] = "False", "Failed"
				}
				last := map[string]any{"outcome": outcome}
				if test.FinishedAt != nil {
					last["finishedAt"] = test.FinishedAt.Format(time.RFC3339Nano)
				}
				value["last"] = last
			case fleet.RestoreOutcomeNotRun, fleet.RestoreOutcomeUnknown:
			}
			value["conditions"] = []any{check}
			backup["restoreTest"] = value
		}
		status["backup"] = backup
	}
}
