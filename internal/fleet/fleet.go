// Package fleet defines the console's domain model: observed OpenBao deployments, the
// connections that read them, and the controllers responsible for them.
//
// JSON tags define the stored observation format. Changing a tag or an enum value changes
// persisted rows. The HTTP API maps these types to its generated contract separately.
package fleet

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// Policy holds the administrator's attention thresholds.
type Policy struct {
	MaxBackupAge time.Duration
	StartupGrace time.Duration
}

// Snapshot is a coherent view of the fleet at one time.
type Snapshot struct {
	CollectedAt   time.Time      `json:"collectedAt"`
	Policy        Policy         `json:"-"`
	Connections   []Connection   `json:"connections"`
	Deployments   []Deployment   `json:"deployments"`
	Relationships []Relationship `json:"relationships"`
	Activity      []Activity     `json:"activity"`
}

// Connection is the collection state of one enrolled cluster.
type Connection struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	Region        string               `json:"region"`
	Collection    ConnectionCollection `json:"collection"`
	LastAttemptAt time.Time            `json:"lastAttemptAt"`
	LastSuccessAt *time.Time           `json:"lastSuccessAt"`
	Namespaces    []string             `json:"namespaces"`
	Operators     []Operator           `json:"operators"`
	Capabilities  []Capability         `json:"capabilities"`
}

// Operator is the observed state of one enrolled controller Deployment.
// Enrollment declares responsibility; workload readiness does not prove reconciliation.
type Operator struct {
	ID                 string             `json:"id"`
	Namespace          string             `json:"namespace"`
	Name               string             `json:"name"`
	Container          string             `json:"container"`
	Namespaces         []string           `json:"namespaces"`
	Collection         OperatorCollection `json:"collection"`
	LastAttemptAt      time.Time          `json:"lastAttemptAt"`
	LastSuccessAt      *time.Time         `json:"lastSuccessAt"`
	Health             OperatorHealth     `json:"health"`
	UID                *string            `json:"uid"`
	Image              *string            `json:"image"`
	Version            *string            `json:"version"`
	Generation         int64              `json:"generation"`
	ObservedGeneration *int64             `json:"observedGeneration"`
	DesiredReplicas    *int32             `json:"desiredReplicas"`
	ReadyReplicas      *int32             `json:"readyReplicas"`
	UpdatedReplicas    *int32             `json:"updatedReplicas"`
}

// Capability records whether one optional or required API could be read in a namespace.
type Capability struct {
	Namespace string             `json:"namespace"`
	Resource  CapabilityResource `json:"resource"`
	State     CapabilityState    `json:"state"`
	CheckedAt *time.Time         `json:"checkedAt"`
}

// Deployment is the approved projection of one OpenBaoCluster and its evaluation.
type Deployment struct {
	ID              string            `json:"id"`
	UID             string            `json:"uid"`
	Name            string            `json:"name"`
	Namespace       string            `json:"namespace"`
	ConnectionID    string            `json:"connectionId"`
	Team            string            `json:"team"`
	Environment     Environment       `json:"environment"`
	Location        string            `json:"location"`
	Generation      int64             `json:"generation"`
	ResourceVersion string            `json:"resourceVersion"`
	Version         string            `json:"version"`
	DesiredVersion  string            `json:"desiredVersion"`
	ReadyReplicas   *int32            `json:"readyReplicas"`
	DesiredReplicas *int32            `json:"desiredReplicas"`
	Role            Role              `json:"role"`
	Health          Health            `json:"health"`
	OperatorID      *string           `json:"operatorId"`
	OperatorVersion *string           `json:"operatorVersion"`
	Conditions      []Condition       `json:"conditions"`
	Issues          []Issue           `json:"issues"`
	Evidence        Evidence          `json:"evidence"`
	UpgradeFailure  *OperationFailure `json:"upgradeFailure"`
	Backup          *Backup           `json:"backup"`
	Replica         *Replica          `json:"replica"`
	Recovery        *Recovery         `json:"recovery"`
}

// Condition is an operator status condition. The collector never retains raw operator
// messages and leaves Message empty; the reason is shown instead.
type Condition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	Reason             string          `json:"reason"`
	Message            string          `json:"message"`
	ObservedGeneration int64           `json:"observedGeneration"`
	LastTransitionAt   *time.Time      `json:"lastTransitionAt"`
}

// Issue is a finding produced by an evidence rule.
type Issue struct {
	ID            string    `json:"id"`
	DeploymentID  string    `json:"deploymentId"`
	Rule          Rule      `json:"rule"`
	Severity      Severity  `json:"severity"`
	Category      Category  `json:"category"`
	Title         string    `json:"title"`
	Detail        string    `json:"detail"`
	FirstSeenAt   time.Time `json:"firstSeenAt"`
	LastCheckedAt time.Time `json:"lastCheckedAt"`
}

// Evidence separates collection time from the producer's own observation time.
type Evidence struct {
	Collection         EvidenceCollection `json:"collection"`
	CollectedAt        time.Time          `json:"collectedAt"`
	LastAttemptAt      time.Time          `json:"lastAttemptAt"`
	ProducerFreshness  Freshness          `json:"producerFreshness"`
	ProducerObservedAt *time.Time         `json:"producerObservedAt"`
}

type OperationFailure struct {
	Reason string     `json:"reason"`
	At     *time.Time `json:"at"`
}

type Backup struct {
	LastSuccessAt       *time.Time   `json:"lastSuccessAt"`
	NextAt              *time.Time   `json:"nextAt"`
	Key                 *string      `json:"key"`
	ConsecutiveFailures int32        `json:"consecutiveFailures"`
	Digest              *string      `json:"digest"`
	SizeBytes           *int64       `json:"sizeBytes"`
	RestoreTest         *RestoreTest `json:"restoreTest"`
}

type RestoreTest struct {
	Outcome       RestoreOutcome `json:"outcome"`
	LastSuccessAt *time.Time     `json:"lastSuccessAt"`
	FinishedAt    *time.Time     `json:"finishedAt"`
	Key           *string        `json:"key"`
	Reason        string         `json:"reason"`
	Digest        *string        `json:"digest"`
}

// Replica is the operator's remote replica observation. An applied index does not
// authorize survivor selection.
type Replica struct {
	ObservedAt      *time.Time `json:"observedAt"`
	HistoryConflict bool       `json:"historyConflict"`
	LocalIndex      *string    `json:"localIndex"`
	SourceIndex     *string    `json:"sourceIndex"`
}

type Recovery struct {
	RequestName            string            `json:"requestName"`
	Mechanism              RecoveryMechanism `json:"mechanism"`
	Phase                  RecoveryPhase     `json:"phase"`
	Stage                  string            `json:"stage"`
	StartedAt              *time.Time        `json:"startedAt"`
	CompletedAt            *time.Time        `json:"completedAt"`
	Cleanup                RecoveryCleanup   `json:"cleanup"`
	ApplicationConfirmedAt *time.Time        `json:"applicationConfirmedAt"`
	ExclusionAcknowledged  *bool             `json:"exclusionAcknowledged"`
}

// Relationship links two deployments. Declared relationships come from enrollment.
type Relationship struct {
	ID         string           `json:"id"`
	SourceID   string           `json:"sourceId"`
	TargetID   string           `json:"targetId"`
	Kind       RelationshipKind `json:"kind"`
	Repository *string          `json:"repository"`
	Provenance Provenance       `json:"provenance"`
	ObservedAt *time.Time       `json:"observedAt"`
}

// Activity records a visible state transition of one deployment.
type Activity struct {
	ID           string          `json:"id"`
	DeploymentID string          `json:"deploymentId"`
	At           time.Time       `json:"at"`
	Title        string          `json:"title"`
	Detail       string          `json:"detail"`
	Outcome      ActivityOutcome `json:"outcome"`
}

// Observation is one retained deployment projection.
type Observation struct {
	RecordedAt time.Time  `json:"recordedAt"`
	Deployment Deployment `json:"deployment"`
}

// DeploymentID derives a stable identifier from the resource identity. A replacement
// resource with a new UID receives a new identifier and never inherits history.
func DeploymentID(connection, namespace, name, uid string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{connection, namespace, name, uid}, "\x00")))
	return hex.EncodeToString(sum[:16])
}

// AssignOperator links a deployment to the enrolled controller responsible for its
// namespace. Responsibility comes from enrollment, never from resource labels.
func AssignOperator(deployment *Deployment, operators []Operator) {
	deployment.OperatorID, deployment.OperatorVersion = nil, nil
	for _, operator := range operators {
		if slices.Contains(operator.Namespaces, deployment.Namespace) {
			id := operator.ID
			deployment.OperatorID, deployment.OperatorVersion = &id, operator.Version
			return
		}
	}
}
