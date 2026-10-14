package fleet

// ConnectionCollection is the result of the latest collection round for a connection.
type ConnectionCollection string

const (
	ConnectionCollectionCurrent     ConnectionCollection = "current"
	ConnectionCollectionPending     ConnectionCollection = "pending"
	ConnectionCollectionStale       ConnectionCollection = "stale"
	ConnectionCollectionDenied      ConnectionCollection = "denied"
	ConnectionCollectionUnreachable ConnectionCollection = "unreachable"
	ConnectionCollectionUnsupported ConnectionCollection = "unsupported"
)

// EvidenceCollection describes whether a deployment's facts are current or last known.
type EvidenceCollection string

const (
	EvidenceCollectionCurrent     EvidenceCollection = "current"
	EvidenceCollectionStale       EvidenceCollection = "stale"
	EvidenceCollectionDenied      EvidenceCollection = "denied"
	EvidenceCollectionUnreachable EvidenceCollection = "unreachable"
	EvidenceCollectionUnsupported EvidenceCollection = "unsupported"
)

// OperatorCollection is the result of reading one enrolled controller Deployment.
type OperatorCollection string

const (
	OperatorCollectionCurrent     OperatorCollection = "current"
	OperatorCollectionPending     OperatorCollection = "pending"
	OperatorCollectionStale       OperatorCollection = "stale"
	OperatorCollectionDenied      OperatorCollection = "denied"
	OperatorCollectionUnreachable OperatorCollection = "unreachable"
	OperatorCollectionNotFound    OperatorCollection = "notFound"
	OperatorCollectionInvalid     OperatorCollection = "invalid"
)

type OperatorHealth string

const (
	OperatorHealthAvailable   OperatorHealth = "available"
	OperatorHealthProgressing OperatorHealth = "progressing"
	OperatorHealthUnavailable OperatorHealth = "unavailable"
	OperatorHealthUnknown     OperatorHealth = "unknown"
)

type CapabilityResource string

const (
	CapabilityResourceClusters   CapabilityResource = "openbaoclusters"
	CapabilityResourceRestores   CapabilityResource = "openbaorestores"
	CapabilityResourceRecoveries CapabilityResource = "openbaorecoveries"
)

type CapabilityState string

const (
	CapabilityStateAvailable   CapabilityState = "available"
	CapabilityStateNotChecked  CapabilityState = "notChecked"
	CapabilityStateDenied      CapabilityState = "denied"
	CapabilityStateUnreachable CapabilityState = "unreachable"
	CapabilityStateUnsupported CapabilityState = "unsupported"
)

// Health summarizes a deployment's findings and workload readiness.
type Health string

const (
	HealthHealthy   Health = "healthy"
	HealthAttention Health = "attention"
	HealthCritical  Health = "critical"
	HealthUnknown   Health = "unknown"
)

type Environment string

const (
	EnvironmentProduction  Environment = "production"
	EnvironmentStaging     Environment = "staging"
	EnvironmentDevelopment Environment = "development"
	EnvironmentOther       Environment = "other"
)

func (e Environment) Valid() bool {
	switch e {
	case EnvironmentProduction, EnvironmentStaging, EnvironmentDevelopment, EnvironmentOther:
		return true
	}
	return false
}

type Role string

const (
	RolePrimary        Role = "primary"
	RoleReplica        Role = "replica"
	RoleRecoveryTarget Role = "recoveryTarget"
)

type ConditionStatus string

const (
	ConditionStatusTrue    ConditionStatus = "True"
	ConditionStatusFalse   ConditionStatus = "False"
	ConditionStatusUnknown ConditionStatus = "Unknown"
)

func (s ConditionStatus) Valid() bool {
	switch s {
	case ConditionStatusTrue, ConditionStatusFalse, ConditionStatusUnknown:
		return true
	}
	return false
}

type Freshness string

const (
	FreshnessFresh   Freshness = "fresh"
	FreshnessStale   Freshness = "stale"
	FreshnessUnknown Freshness = "unknown"
)

// Rule identifies the evidence rule that produced an issue.
type Rule string

const (
	RuleReplicaReadiness       Rule = "replica-readiness"
	RuleSealed                 Rule = "sealed"
	RuleUpgradeFailed          Rule = "upgrade-failed"
	RuleBackupFailed           Rule = "backup-failed"
	RuleBackupNeverSucceeded   Rule = "backup-never-succeeded"
	RuleBackupOverdue          Rule = "backup-overdue"
	RuleRestoreTest            Rule = "restore-test"
	RuleReplicaHistoryConflict Rule = "replica-history-conflict"
	RuleRecoveryInvestigate    Rule = "recovery-investigate"
	RuleCollection             Rule = "collection"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityInfo     Severity = "info"
)

type Category string

const (
	CategoryAvailability Category = "availability"
	CategoryBackup       Category = "backup"
	CategoryOperations   Category = "operations"
	CategoryVisibility   Category = "visibility"
)

type RestoreOutcome string

const (
	RestoreOutcomeNotRun  RestoreOutcome = "notRun"
	RestoreOutcomeRunning RestoreOutcome = "running"
	RestoreOutcomePassed  RestoreOutcome = "passed"
	RestoreOutcomeFailed  RestoreOutcome = "failed"
	RestoreOutcomeBlocked RestoreOutcome = "blocked"
)

type RecoveryMechanism string

const (
	RecoveryMechanismSnapshot RecoveryMechanism = "snapshot"
	RecoveryMechanismNative   RecoveryMechanism = "native"
)

type RecoveryPhase string

const (
	RecoveryPhasePending    RecoveryPhase = "pending"
	RecoveryPhaseValidating RecoveryPhase = "validating"
	RecoveryPhaseRunning    RecoveryPhase = "running"
	RecoveryPhaseCompleted  RecoveryPhase = "completed"
	RecoveryPhaseFailed     RecoveryPhase = "failed"
	RecoveryPhaseUnknown    RecoveryPhase = "unknown"
)

func (p RecoveryPhase) Valid() bool {
	switch p {
	case RecoveryPhasePending, RecoveryPhaseValidating, RecoveryPhaseRunning, RecoveryPhaseCompleted, RecoveryPhaseFailed,
		RecoveryPhaseUnknown:
		return true
	}
	return false
}

type RecoveryCleanup string

const (
	RecoveryCleanupNotApplicable RecoveryCleanup = "notApplicable"
	RecoveryCleanupPending       RecoveryCleanup = "pending"
	RecoveryCleanupComplete      RecoveryCleanup = "complete"
	RecoveryCleanupFailed        RecoveryCleanup = "failed"
)

func (c RecoveryCleanup) Valid() bool {
	switch c {
	case RecoveryCleanupNotApplicable, RecoveryCleanupPending, RecoveryCleanupComplete, RecoveryCleanupFailed:
		return true
	}
	return false
}

type RelationshipKind string

const (
	RelationshipKindReplication      RelationshipKind = "replication"
	RelationshipKindSnapshotRecovery RelationshipKind = "snapshotRecovery"
)

func (k RelationshipKind) Valid() bool {
	return k == RelationshipKindReplication || k == RelationshipKindSnapshotRecovery
}

type Provenance string

const (
	ProvenanceDeclared   Provenance = "declared"
	ProvenanceObserved   Provenance = "observed"
	ProvenanceHistorical Provenance = "historical"
)

// ActivityKind names the transition an activity event records.
type ActivityKind string

const (
	ActivityHealthChanged          ActivityKind = "healthChanged"
	ActivityCollectionChanged      ActivityKind = "collectionChanged"
	ActivityFindingRaised          ActivityKind = "findingRaised"
	ActivityFindingResolved        ActivityKind = "findingResolved"
	ActivityFindingSeverityChanged ActivityKind = "findingSeverityChanged"
	ActivityLegacy                 ActivityKind = "legacy"
)
