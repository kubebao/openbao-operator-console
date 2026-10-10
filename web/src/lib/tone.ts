import type { Deployment, Operator } from '../api/schema'

type RestoreOutcome = NonNullable<NonNullable<Deployment['backup']>['restoreTest']>['outcome']

// Tone selects a status color through the status-* classes.
export type Tone = 'healthy' | 'attention' | 'critical' | 'unknown'

export const operatorTone: Record<Operator['health'], Tone> = {
  available: 'healthy',
  progressing: 'attention',
  unavailable: 'critical',
  unknown: 'unknown',
}

export const restoreTone: Record<RestoreOutcome, Tone> = {
  passed: 'healthy',
  failed: 'critical',
  blocked: 'attention',
  running: 'unknown',
  notRun: 'unknown',
}
