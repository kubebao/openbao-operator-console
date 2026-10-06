import type { Deployment, History } from '../api/schema'
import { assessCondition } from './conditions'
import {
  bytes,
  cleanupStates,
  collectionLabels,
  healthLabels,
  recoveryPhases,
  restoreOutcomes,
} from './format'

export type Observation = History['observations'][number]
export type ChangeTone = 'critical' | 'warning' | 'success' | 'neutral'

export interface Change {
  title: string
  detail?: string
  tone: ChangeTone
}

export interface HistoryEntry {
  at: string
  observation: Observation
  changes: readonly Change[]
  // True for the oldest retained observation, which has no predecessor to compare with.
  earliest: boolean
  // True when no displayed field changed. Consecutive quiet observations are merged.
  quiet: boolean
}

const healthTone: Record<Deployment['health'], ChangeTone> = {
  critical: 'critical',
  attention: 'warning',
  healthy: 'success',
  unknown: 'neutral',
}

function replicas(value: number | null): string {
  return value === null ? 'unknown' : String(value)
}

// Describes what changed between two consecutive observations of the same deployment.
// Counters such as consecutive backup failures can change by more than one between
// observations, so their entries describe the observed change, not individual attempts.
export function describeChange(previous: Deployment, current: Deployment): Change[] {
  const changes: Change[] = []
  if (previous.health !== current.health)
    changes.push({
      title: `State ${healthLabels[previous.health]} → ${healthLabels[current.health]}`,
      tone: healthTone[current.health],
    })
  if (previous.evidence.collection !== current.evidence.collection)
    changes.push({
      title: `Collection ${collectionLabels[previous.evidence.collection]} → ${collectionLabels[current.evidence.collection]}`,
      tone: current.evidence.collection === 'current' ? 'success' : 'warning',
    })

  const before = new Map(previous.issues.map((issue) => [issue.rule, issue]))
  const after = new Map(current.issues.map((issue) => [issue.rule, issue]))
  for (const [rule, issue] of after)
    if (!before.has(rule))
      changes.push({
        title: `Finding raised: ${issue.title}`,
        detail: issue.detail,
        tone: issue.severity === 'critical' ? 'critical' : 'warning',
      })
  for (const [rule, issue] of before)
    if (!after.has(rule))
      changes.push({ title: `Finding resolved: ${issue.title}`, tone: 'success' })

  if (previous.desiredReplicas !== current.desiredReplicas)
    changes.push({
      title: `Desired replicas ${replicas(previous.desiredReplicas)} → ${replicas(current.desiredReplicas)}`,
      tone: 'neutral',
    })
  if (previous.readyReplicas !== current.readyReplicas)
    changes.push({
      title: `Ready replicas ${replicas(previous.readyReplicas)} → ${replicas(current.readyReplicas)}`,
      tone:
        current.readyReplicas !== null &&
        current.desiredReplicas !== null &&
        current.readyReplicas >= current.desiredReplicas
          ? 'success'
          : 'warning',
    })
  if (previous.desiredVersion !== current.desiredVersion)
    changes.push({
      title: `Version requested: ${previous.desiredVersion || 'none'} → ${current.desiredVersion || 'none'}`,
      tone: 'neutral',
    })
  if (previous.version !== current.version)
    changes.push({
      title: `Running version ${previous.version || 'unknown'} → ${current.version || 'unknown'}`,
      tone: current.version === current.desiredVersion ? 'success' : 'neutral',
    })
  if (previous.generation !== current.generation)
    changes.push({
      title: 'Specification changed',
      detail: `Generation ${String(previous.generation)} → ${String(current.generation)}`,
      tone: 'neutral',
    })

  const conditions = new Map(previous.conditions.map((condition) => [condition.type, condition]))
  for (const condition of current.conditions) {
    const old = conditions.get(condition.type)
    if (old?.status === condition.status) {
      if (old.reason !== condition.reason)
        changes.push({
          title: `${condition.type} reason ${old.reason || 'none'} → ${condition.reason || 'none'}`,
          tone: 'neutral',
        })
      else if (old.lastTransitionAt !== condition.lastTransitionAt)
        // The status changed and changed back between two collections.
        changes.push({
          title: `${condition.type} changed and returned to ${condition.status}`,
          ...(condition.reason ? { detail: condition.reason } : {}),
          tone: 'neutral',
        })
      continue
    }
    const assessment = assessCondition(condition, current.generation).assessment
    changes.push({
      title: old
        ? `${condition.type} ${old.status} → ${condition.status}`
        : `${condition.type} reported ${condition.status}`,
      ...(condition.message ? { detail: condition.message } : {}),
      tone: assessment === 'problem' ? 'warning' : assessment === 'normal' ? 'success' : 'neutral',
    })
  }
  const remaining = new Set(current.conditions.map((condition) => condition.type))
  for (const condition of previous.conditions)
    if (!remaining.has(condition.type))
      changes.push({ title: `${condition.type} no longer reported`, tone: 'neutral' })

  const oldBackup = previous.backup
  const newBackup = current.backup
  if (newBackup?.lastSuccessAt && newBackup.lastSuccessAt !== oldBackup?.lastSuccessAt)
    changes.push({
      title: 'Backup completed',
      detail: [newBackup.sizeBytes === null ? null : bytes(newBackup.sizeBytes), newBackup.key]
        .filter(Boolean)
        .join(' · '),
      tone: 'success',
    })
  else if (newBackup?.nextAt && oldBackup && newBackup.nextAt !== oldBackup.nextAt)
    changes.push({ title: 'Backup schedule updated', tone: 'neutral' })
  if (newBackup && newBackup.consecutiveFailures > (oldBackup?.consecutiveFailures ?? 0)) {
    const added = newBackup.consecutiveFailures - (oldBackup?.consecutiveFailures ?? 0)
    changes.push({
      title: added === 1 ? 'Backup failed' : `${String(added)} backup failures observed`,
      detail: `${String(newBackup.consecutiveFailures)} consecutive ${newBackup.consecutiveFailures === 1 ? 'failure' : 'failures'}`,
      tone: 'warning',
    })
  }
  const oldTest = oldBackup?.restoreTest ?? null
  const newTest = newBackup?.restoreTest ?? null
  if (newTest?.finishedAt && newTest.finishedAt !== oldTest?.finishedAt)
    changes.push({
      title: `Restore test ${restoreOutcomes[newTest.outcome].toLocaleLowerCase()}`,
      ...(newTest.reason ? { detail: newTest.reason } : {}),
      tone:
        newTest.outcome === 'passed'
          ? 'success'
          : newTest.outcome === 'failed' || newTest.outcome === 'blocked'
            ? 'warning'
            : 'neutral',
    })

  if (current.upgradeFailure && !previous.upgradeFailure)
    changes.push({ title: `Upgrade failed: ${current.upgradeFailure.reason}`, tone: 'warning' })
  if (!current.upgradeFailure && previous.upgradeFailure)
    changes.push({ title: 'Upgrade failure cleared', tone: 'success' })

  const oldRecovery = previous.recovery
  const newRecovery = current.recovery
  if (newRecovery && newRecovery.requestName !== oldRecovery?.requestName)
    changes.push({
      title: `Recovery requested: ${newRecovery.requestName}`,
      detail: `Phase ${recoveryPhases[newRecovery.phase]}`,
      tone: 'neutral',
    })
  else if (newRecovery && oldRecovery && newRecovery.phase !== oldRecovery.phase)
    changes.push({
      title: `Recovery ${recoveryPhases[oldRecovery.phase]} → ${recoveryPhases[newRecovery.phase]}`,
      tone:
        newRecovery.phase === 'completed'
          ? 'success'
          : newRecovery.phase === 'failed' || newRecovery.phase === 'unknown'
            ? 'warning'
            : 'neutral',
    })
  if (newRecovery && oldRecovery && newRecovery.cleanup !== oldRecovery.cleanup)
    changes.push({
      title: `Recovery cleanup ${cleanupStates[newRecovery.cleanup].toLocaleLowerCase()}`,
      tone: newRecovery.cleanup === 'failed' ? 'warning' : 'neutral',
    })

  if (current.replica?.historyConflict && !previous.replica?.historyConflict)
    changes.push({ title: 'Replica history conflict recorded', tone: 'critical' })
  if (previous.replica?.historyConflict && !current.replica?.historyConflict)
    changes.push({ title: 'Replica history conflict cleared', tone: 'success' })
  return changes
}

function ascending(observations: readonly Observation[]): Observation[] {
  return [...observations].sort((a, b) => Date.parse(a.recordedAt) - Date.parse(b.recordedAt))
}

// Turns retained observations into change entries, newest first. Observations without a
// displayed change are kept as quiet entries, and consecutive quiet entries are merged so
// status-only updates recorded by older versions stay out of the way.
export function describeHistory(observations: readonly Observation[]): HistoryEntry[] {
  const sorted = ascending(observations)
  const entries: HistoryEntry[] = []
  let quietRun: { first: Observation; count: number } | null = null
  sorted.forEach((observation, index) => {
    const previous = sorted[index - 1]
    if (!previous) {
      entries.push({
        at: observation.recordedAt,
        observation,
        earliest: true,
        quiet: false,
        changes: [
          {
            title: 'Earliest retained observation',
            detail: `State ${healthLabels[observation.deployment.health]}`,
            tone: 'neutral',
          },
        ],
      })
      return
    }
    const changes = describeChange(previous.deployment, observation.deployment)
    if (changes.length) {
      quietRun = null
      entries.push({
        at: observation.recordedAt,
        observation,
        earliest: false,
        quiet: false,
        changes,
      })
      return
    }
    const last = entries.at(-1)
    quietRun =
      quietRun && last?.quiet
        ? { ...quietRun, count: quietRun.count + 1 }
        : { first: observation, count: 1 }
    const entry: HistoryEntry = {
      at: observation.recordedAt,
      observation,
      earliest: false,
      quiet: true,
      changes: [
        {
          title:
            quietRun.count === 1
              ? 'Status updated without visible changes'
              : `Status updated ${String(quietRun.count)} times without visible changes`,
          detail: `Resource version ${quietRun.first.deployment.resourceVersion}${quietRun.count > 1 ? ` → ${observation.deployment.resourceVersion}` : ''}`,
          tone: 'neutral',
        },
      ],
    }
    if (quietRun.count > 1 && last?.quiet) entries[entries.length - 1] = entry
    else entries.push(entry)
  })
  return entries.reverse()
}

export type BandState = Deployment['health'] | 'uncollected' | 'unrecorded'

export const bandLabels: Record<BandState, string> = {
  healthy: 'Healthy',
  attention: 'Attention',
  critical: 'Critical',
  unknown: 'Unknown',
  uncollected: 'Collection failed',
  unrecorded: 'Not recorded',
}

export interface BandSegment {
  start: number
  end: number
  state: BandState
}

// Recorded state over a time window. Observations are stored only when something changes,
// so each state lasts until the next observation. Time before the oldest retained
// observation is unrecorded, and failed collection is shown as uncollected.
export function stateBand(
  observations: readonly Observation[],
  windowStart: number,
  windowEnd: number,
): BandSegment[] {
  const sorted = ascending(observations)
  const segments: BandSegment[] = []
  const first = sorted[0]
  const firstAt = first ? Date.parse(first.recordedAt) : windowEnd
  if (firstAt > windowStart)
    segments.push({ start: windowStart, end: Math.min(firstAt, windowEnd), state: 'unrecorded' })
  sorted.forEach((observation, index) => {
    const next = sorted[index + 1]
    const start = Math.max(Date.parse(observation.recordedAt), windowStart)
    const end = Math.min(next ? Date.parse(next.recordedAt) : windowEnd, windowEnd)
    if (end <= start) return
    const state: BandState =
      observation.deployment.evidence.collection === 'current'
        ? observation.deployment.health
        : 'uncollected'
    const last = segments.at(-1)
    if (last?.state === state && last.end === start) last.end = end
    else segments.push({ start, end, state })
  })
  return segments
}

export interface BackupEvent {
  at: string
  kind: 'success' | 'failure'
  sizeBytes: number | null
  // Failures observed in this change; counters can move by more than one between observations.
  count: number
}

// Backup completions and observed failures, oldest first, from retained observations.
export function backupEvents(observations: readonly Observation[]): BackupEvent[] {
  const events: BackupEvent[] = []
  const completed = new Set<string>()
  let failures = 0
  for (const observation of ascending(observations)) {
    const backup = observation.deployment.backup
    if (!backup) continue
    if (backup.lastSuccessAt && !completed.has(backup.lastSuccessAt)) {
      completed.add(backup.lastSuccessAt)
      events.push({
        at: backup.lastSuccessAt,
        kind: 'success',
        sizeBytes: backup.sizeBytes,
        count: 1,
      })
    }
    if (backup.consecutiveFailures > failures)
      events.push({
        at: observation.recordedAt,
        kind: 'failure',
        sizeBytes: null,
        count: backup.consecutiveFailures - failures,
      })
    failures = backup.consecutiveFailures
  }
  return events.sort((a, b) => Date.parse(a.at) - Date.parse(b.at))
}

// When each condition last changed status, from retained observations.
export function conditionTransitions(
  observations: readonly Observation[],
): Map<string, { from: string; at: string }> {
  const result = new Map<string, { from: string; at: string }>()
  const sorted = ascending(observations)
  sorted.forEach((observation, index) => {
    const previous = sorted[index - 1]
    if (!previous) return
    const before = new Map(previous.deployment.conditions.map((item) => [item.type, item.status]))
    for (const condition of observation.deployment.conditions) {
      const status = before.get(condition.type)
      if (status !== undefined && status !== condition.status)
        result.set(condition.type, { from: status, at: observation.recordedAt })
    }
  })
  return result
}
