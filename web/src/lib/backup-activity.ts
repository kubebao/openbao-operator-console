import type { BackupEvent } from './history'

const hour = 60 * 60 * 1000

// An hour cell of the backup week. Failure outranks completion. Without a backup event, an
// hour before the oldest retained observation is not recorded rather than empty.
export type HourState = 'failed' | 'completed' | 'none' | 'unrecorded' | 'unloaded' | 'future'

export interface HourCell {
  start: number
  end: number
  state: HourState
  completed: number
  failures: number
}

export interface DayRow {
  start: number
  cells: HourCell[]
}

function hourState(
  future: boolean,
  completed: number,
  failures: number,
  recorded: boolean,
  earlierUnloaded: boolean,
): HourState {
  if (future) return 'future'
  if (failures > 0) return 'failed'
  if (completed > 0) return 'completed'
  if (!recorded) return earlierUnloaded ? 'unloaded' : 'unrecorded'
  return 'none'
}

// backupWeek lays out the last `days` local days as rows of hour cells, ending with today.
// `recordedFrom` is the oldest retained observation, or null when none is retained.
export function backupWeek(
  events: readonly BackupEvent[],
  now: number,
  recordedFrom: number | null,
  earlierUnloaded = false,
  days = 7,
): DayRow[] {
  const rows: DayRow[] = []
  const today = new Date(now)
  for (let offset = days - 1; offset >= 0; offset -= 1) {
    const day = new Date(today.getFullYear(), today.getMonth(), today.getDate() - offset)
    const cells: HourCell[] = []
    for (let index = 0; index < 24; index += 1) {
      const start = new Date(day.getFullYear(), day.getMonth(), day.getDate(), index).getTime()
      const end = start + hour
      let completed = 0
      let failures = 0
      for (const event of events) {
        const at = Date.parse(event.at)
        if (at < start || at >= end) continue
        if (event.kind === 'success') completed += event.count
        else failures += event.count
      }
      const recorded = recordedFrom !== null && end > recordedFrom
      const state = hourState(start > now, completed, failures, recorded, earlierUnloaded)
      cells.push({ start, end, completed, failures, state })
    }
    rows.push({ start: day.getTime(), cells })
  }
  return rows
}

// Where the backup schedule stands against the maximum backup age policy.
export type BackupTiming =
  | { kind: 'none' }
  | { kind: 'overdue'; overBy: number }
  | { kind: 'unscheduled'; remaining: number }
  | { kind: 'late'; nextIn: number }
  | { kind: 'missed'; dueAgo: number; remaining: number }
  | { kind: 'onTime'; nextIn: number }

// backupTiming compares the last success, the next reported run, and the policy limit. All
// durations are milliseconds.
export function backupTiming(
  lastSuccessAt: string | null,
  nextAt: string | null,
  limitHours: number,
  now: number,
): BackupTiming {
  if (lastSuccessAt === null) return { kind: 'none' }
  const deadline = Date.parse(lastSuccessAt) + limitHours * hour
  if (now > deadline) return { kind: 'overdue', overBy: now - deadline }
  if (nextAt === null) return { kind: 'unscheduled', remaining: deadline - now }
  const next = Date.parse(nextAt)
  if (next < now) return { kind: 'missed', dueAgo: now - next, remaining: deadline - now }
  if (next > deadline) return { kind: 'late', nextIn: next - now }
  return { kind: 'onTime', nextIn: next - now }
}

export interface SizeTrend {
  count: number
  first: { at: number; value: number }
  last: { at: number; value: number }
  // Whether any retained size differs from the others.
  varies: boolean
}

// sizeTrend summarises the sizes of retained successful backups, oldest first.
export function sizeTrend(points: readonly { at: number; value: number }[]): SizeTrend | null {
  const first = points[0]
  const last = points.at(-1)
  if (!first || !last || points.length < 2) return null
  return {
    count: points.length,
    first,
    last,
    varies: points.some((point) => point.value !== first.value),
  }
}
