import type { Deployment, Snapshot } from '../api/schema'

function span(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${String(minutes)}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${String(hours)}h`
  const days = Math.floor(hours / 24)
  if (days < 365) return `${String(days)}d`
  return `${String(Math.floor(days / 365))}y`
}

// Observation times describe the past. A future observation is a clock problem, not freshness.
export function age(value: string | null, now = Date.now()): string {
  if (value === null) return 'Not observed'
  const seconds = Math.floor((now - Date.parse(value)) / 1000)
  if (!Number.isFinite(seconds)) return 'Unknown'
  if (seconds < -5) return 'Clock ahead'
  if (seconds < 60) return 'Just now'
  return `${span(seconds)} ago`
}

// Scheduled times such as expiry or the next attempt usually describe the future.
export function until(value: string | null, now = Date.now()): string {
  if (value === null) return 'Not scheduled'
  const seconds = Math.floor((Date.parse(value) - now) / 1000)
  if (!Number.isFinite(seconds)) return 'Unknown'
  if (Math.abs(seconds) < 60) return 'Now'
  return seconds > 0 ? `in ${span(seconds)}` : `${span(-seconds)} ago`
}

const absoluteFormat = new Intl.DateTimeFormat(undefined, {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  timeZoneName: 'short',
})

export function absolute(value: string | number): string {
  const time = typeof value === 'number' ? value : Date.parse(value)
  return Number.isFinite(time) ? absoluteFormat.format(time) : 'Unknown'
}

export function bytes(value: number | null): string {
  if (value === null) return 'Not observed'
  if (value < 1024) return `${String(value)} B`
  if (value < 1024 ** 2) return `${(value / 1024).toFixed(1)} KiB`
  if (value < 1024 ** 3) return `${(value / 1024 ** 2).toFixed(1)} MiB`
  return `${(value / 1024 ** 3).toFixed(1)} GiB`
}

export const roleLabels = {
  primary: 'Primary',
  replica: 'Read replica',
  recoveryTarget: 'Recovery target',
}
export const healthLabels: Record<Deployment['health'], string> = {
  healthy: 'Healthy',
  attention: 'Attention',
  critical: 'Critical',
  unknown: 'Unknown',
}
// Critical first, then unassessed deployments, so the fleet reads as a triage list.
export const healthRank: Record<Deployment['health'], number> = {
  critical: 0,
  attention: 1,
  unknown: 2,
  healthy: 3,
}
export const environmentLabels: Record<Deployment['environment'], string> = {
  production: 'Production',
  staging: 'Staging',
  development: 'Development',
  other: 'Other',
}
export const categoryLabels = {
  availability: 'Service availability',
  operations: 'Operations',
  backup: 'Backup protection',
  visibility: 'Collection',
}

export type CollectionState = Snapshot['connections'][number]['collection']
export const collectionLabels: Record<CollectionState, string> = {
  current: 'Collected',
  pending: 'Awaiting collection',
  stale: 'Stale',
  denied: 'Access denied',
  unreachable: 'Unreachable',
  unsupported: 'Unsupported',
}

export const operatorCheckLabels = {
  current: 'Accessible',
  pending: 'Awaiting collection',
  stale: 'Stale',
  denied: 'Access denied',
  unreachable: 'Unreachable',
  notFound: 'Deployment missing',
  invalid: 'Check container enrollment',
} as const

export const capabilityCheckLabels = {
  available: 'Accessible',
  denied: 'Access denied',
  unsupported: 'API unavailable',
  notChecked: 'Not checked',
  unreachable: 'Unreachable',
} as const

export const operatorHealthLabels = {
  available: 'Available',
  progressing: 'Progressing',
  unavailable: 'Unavailable',
  unknown: 'Unknown',
} as const

export const restoreOutcomes = {
  notRun: 'Not run',
  running: 'Running',
  passed: 'Passed',
  failed: 'Failed',
  blocked: 'Blocked',
} as const
export const recoveryPhases = {
  pending: 'Pending',
  validating: 'Validating',
  running: 'Running',
  completed: 'Completed',
  failed: 'Failed',
  unknown: 'Unknown',
} as const
export const cleanupStates = {
  notApplicable: 'Not applicable',
  pending: 'Pending',
  complete: 'Complete',
  failed: 'Failed',
} as const

export const provenanceLabels = {
  declared: 'Declared',
  observed: 'Observed',
  historical: 'Historical',
} as const
