import type { Deployment, Operator } from '../api/schema'

type RestoreOutcome = NonNullable<NonNullable<Deployment['backup']>['restoreTest']>['outcome']

// Tone selects a status color through the status-* classes.
export type Tone = 'healthy' | 'attention' | 'critical' | 'unknown'

// operatorNeedsAttention reports an enrolled operator without current, available evidence.
export function operatorNeedsAttention(operator: Pick<Operator, 'collection' | 'health'>): boolean {
  return operator.collection !== 'current' || operator.health !== 'available'
}

export const operatorTone: Record<Operator['health'], Tone> = {
  available: 'healthy',
  progressing: 'attention',
  unavailable: 'critical',
  unknown: 'unknown',
}

export const producerFreshnessTone: Record<Deployment['evidence']['producerFreshness'], Tone> = {
  fresh: 'healthy',
  stale: 'attention',
  unknown: 'unknown',
}

export const restoreTone: Record<RestoreOutcome, Tone> = {
  passed: 'healthy',
  failed: 'critical',
  blocked: 'attention',
  running: 'unknown',
  notRun: 'unknown',
}
