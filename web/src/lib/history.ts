import type { Deployment, History } from '../api/schema'
import type { Change } from './changes'
import { describeChange } from './changes'
import { healthLabels } from './format'

export type Observation = History['observations'][number]
export interface HistoryEntry {
  at: string
  observation: Observation
  changes: readonly Change[]
  // True for the oldest retained observation, which has no predecessor to compare with.
  earliest: boolean
  // True when no displayed field changed. Consecutive quiet observations are merged.
  quiet: boolean
}

function ascending(observations: readonly Observation[]): Observation[] {
  return [...observations]
    .reverse()
    .sort((a, b) => Date.parse(a.recordedAt) - Date.parse(b.recordedAt))
}

// Turns retained observations into change entries, newest first. Observations without a
// displayed change are kept as quiet entries, and consecutive quiet entries are merged so
// status-only updates recorded by older versions stay out of the way.
export function describeHistory(
  observations: readonly Observation[],
  baseline: Observation | null = null,
  earlierUnloaded = false,
): HistoryEntry[] {
  const sorted = ascending(observations)
  const entries: HistoryEntry[] = []
  let quietRun: { first: Observation; count: number } | null = null
  sorted.forEach((observation, index) => {
    const previous = sorted[index - 1] ?? baseline
    if (!previous) {
      entries.push({
        at: observation.recordedAt,
        observation,
        earliest: true,
        quiet: false,
        changes: [
          {
            title: earlierUnloaded
              ? 'Earlier observations not loaded'
              : 'Earliest retained observation',
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

export type BandState = Deployment['health'] | 'uncollected' | 'unrecorded' | 'unloaded'

// Legend order for the state band.
export const bandStates = [
  'healthy',
  'attention',
  'critical',
  'unknown',
  'uncollected',
  'unrecorded',
  'unloaded',
] as const satisfies readonly BandState[]

export const bandLabels: Record<BandState, string> = {
  healthy: 'Healthy',
  attention: 'Attention',
  critical: 'Critical',
  unknown: 'Unknown',
  uncollected: 'Collection failed',
  unrecorded: 'Not recorded',
  unloaded: 'Not loaded',
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
  earlierUnloaded = false,
): BandSegment[] {
  const sorted = ascending(observations)
  const segments: BandSegment[] = []
  const first = sorted[0]
  const firstAt = first ? Date.parse(first.recordedAt) : windowEnd
  if (firstAt > windowStart)
    segments.push({
      start: windowStart,
      end: Math.min(firstAt, windowEnd),
      state: earlierUnloaded ? 'unloaded' : 'unrecorded',
    })
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
  let failures: number | null = null
  for (const observation of ascending(observations)) {
    const backup = observation.deployment.backup
    if (!backup) {
      failures = null
      continue
    }
    if (backup.lastSuccessAt && !completed.has(backup.lastSuccessAt)) {
      completed.add(backup.lastSuccessAt)
      events.push({
        at: backup.lastSuccessAt,
        kind: 'success',
        sizeBytes: backup.sizeBytes,
        count: 1,
      })
    }
    if (
      backup.consecutiveFailures !== null &&
      failures !== null &&
      backup.consecutiveFailures > failures
    )
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
