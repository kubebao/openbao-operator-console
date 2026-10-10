import type { Deployment } from '../api/schema'

type Condition = Deployment['conditions'][number]

// How a condition's status reads. "positive" types are good when True, "negative" types are
// bad when True, and "activity" types report work in progress.
type Polarity = 'positive' | 'negative' | 'activity' | 'neutral'

// Known condition types from the OpenBao Operator's public API (api/v1alpha1). Unknown types
// are neutral unless their name ends in "Ready".
const polarities: Record<string, Polarity> = {
  Available: 'positive',
  ProductionReady: 'positive',
  OpenBaoInitialized: 'positive',
  StorageConfigured: 'positive',
  ReadServingAvailable: 'positive',
  ReadReplicasAutopilotHealthy: 'positive',
  ReadReplicaStorageConfigured: 'positive',
  Degraded: 'negative',
  OpenBaoSealed: 'negative',
  SecurityRisk: 'negative',
  EtcdEncryptionWarning: 'negative',
  NodeSecurityCapabilityMismatch: 'negative',
  Upgrading: 'activity',
  BackingUp: 'activity',
}

function polarity(type: string): Polarity {
  return polarities[type] ?? (type.endsWith('Ready') ? 'positive' : 'neutral')
}

export type ConditionAssessment = 'problem' | 'unknown' | 'active' | 'normal' | 'informational'

export interface ConditionView {
  condition: Condition
  assessment: ConditionAssessment
  // The operator has not reported this condition for the current resource generation.
  outdated: boolean
}

function assess(condition: Condition): ConditionAssessment {
  const kind = polarity(condition.type)
  if (condition.status === 'Unknown') return kind === 'neutral' ? 'informational' : 'unknown'
  const reported = condition.status === 'True'
  switch (kind) {
    case 'positive':
      return reported ? 'normal' : 'problem'
    case 'negative':
      return reported ? 'problem' : 'normal'
    case 'activity':
      // A finished activity is neutral unless its reason reports that it failed.
      if (reported) return 'active'
      return /fail|error/i.test(condition.reason) ? 'problem' : 'informational'
    case 'neutral':
      return 'informational'
  }
}

export function assessCondition(condition: Condition, generation: number): ConditionView {
  return {
    condition,
    assessment: assess(condition),
    outdated: condition.observedGeneration < generation,
  }
}

const order: Record<ConditionAssessment, number> = {
  problem: 0,
  unknown: 1,
  active: 2,
  informational: 3,
  normal: 4,
}

// Problems first, then uncertain and in-progress conditions; stable by type within a group.
export function assessConditions(deployment: Pick<Deployment, 'conditions' | 'generation'>) {
  return deployment.conditions
    .map((condition) => assessCondition(condition, deployment.generation))
    .sort(
      (a, b) =>
        order[a.assessment] - order[b.assessment] ||
        Number(b.outdated) - Number(a.outdated) ||
        a.condition.type.localeCompare(b.condition.type),
    )
}

export const assessmentLabels: Record<ConditionAssessment, string> = {
  problem: 'Needs attention',
  unknown: 'Unknown',
  active: 'In progress',
  informational: 'Reported',
  normal: 'As expected',
}
