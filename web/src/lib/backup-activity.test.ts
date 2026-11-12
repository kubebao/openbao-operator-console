import { describe, expect, it, vi } from 'vitest'
import type { BackupEvent } from './history'
import { backupTiming, backupWeek, sizeTrend } from './backup-activity'

const hour = 60 * 60 * 1000
// Local time, so day rows and hour cells line up with the browser's calendar.
const now = new Date(2026, 10, 7, 12, 30).getTime()
const at = (day: number, hourOfDay: number, minute = 0) =>
  new Date(2026, 10, day, hourOfDay, minute).toISOString()
const success = (time: string): BackupEvent => ({
  at: time,
  kind: 'success',
  sizeBytes: 1,
  count: 1,
})

describe('backup week', () => {
  it('covers the complete rolling window across its eight local dates', () => {
    const rows = backupWeek([], now, null)
    expect(rows).toHaveLength(8)
    expect(rows.every((row) => row.cells.length === 24)).toBe(true)
    expect(new Date(rows[7]?.start ?? 0).getDate()).toBe(7)
  })
  it('lets a failure outrank a completion in the same hour', () => {
    const rows = backupWeek(
      [
        success(at(7, 4, 5)),
        { at: at(7, 4, 40), kind: 'failure', sizeBytes: null, count: 2 },
        success(at(7, 5, 5)),
      ],
      now,
      Date.parse(at(1, 0)),
    )
    const today = rows[7]?.cells ?? []
    expect(today[4]).toMatchObject({ state: 'failed', completed: 1, failures: 2 })
    expect(today[5]?.state).toBe('completed')
    expect(today[6]?.state).toBe('none')
  })
  it('keeps hours before retained history unrecorded and later hours empty', () => {
    const rows = backupWeek([], now, Date.parse(at(6, 10, 30)))
    const friday = rows[6]?.cells ?? []
    expect(friday[9]?.state).toBe('unrecorded')
    // Coverage that starts inside an hour counts for that hour.
    expect(friday[10]?.state).toBe('none')
    expect(rows[7]?.cells[13]?.state).toBe('future')
  })
  it('marks unloaded hours when older history can still be loaded', () => {
    expect(backupWeek([], now, null, true)[0]?.cells[13]?.state).toBe('unloaded')
  })
  it('clips first-day completions and excludes events after the displayed window', () => {
    const start = now - 7 * 24 * hour
    const events = [
      success(new Date(start - 1).toISOString()),
      success(new Date(start).toISOString()),
      success(new Date(now + 1).toISOString()),
    ]
    const rows = backupWeek(events, now, start)
    expect(
      rows.flatMap((row) => row.cells).reduce((total, cell) => total + cell.completed, 0),
    ).toBe(1)
    expect(rows[0]?.cells[0]?.state).toBe('outside')
  })
  it('retains completions in both occurrences of a repeated local clock hour', () => {
    vi.stubEnv('TZ', 'Europe/Amsterdam')
    try {
      const rows = backupWeek(
        [success('2026-10-25T00:30:00Z'), success('2026-10-25T01:30:00Z')],
        Date.parse('2026-10-25T12:00:00Z'),
        Date.parse('2026-10-18T12:00:00Z'),
      )
      expect(rows.at(-1)?.cells[2]?.completed).toBe(2)
    } finally {
      vi.unstubAllEnvs()
    }
  })
  it('leaves a skipped local clock hour blank without duplicating completions', () => {
    vi.stubEnv('TZ', 'Europe/Amsterdam')
    try {
      const rows = backupWeek(
        [success('2026-03-29T01:30:00Z')],
        Date.parse('2026-03-29T12:00:00Z'),
        Date.parse('2026-03-22T12:00:00Z'),
      )
      const today = rows.at(-1)?.cells ?? []
      expect(today[2]?.state).toBe('outside')
      expect(today[3]?.completed).toBe(1)
      expect(today.reduce((total, cell) => total + cell.completed, 0)).toBe(1)
    } finally {
      vi.unstubAllEnvs()
    }
  })
  it('shows a known completion even before retained history', () => {
    const rows = backupWeek([success(at(2, 3))], now, Date.parse(at(6, 0)))
    expect(rows[2]?.cells[3]?.state).toBe('completed')
  })
})

describe('backup timing', () => {
  const last = new Date(now - 2 * hour).toISOString()
  const inHours = (hours: number) => new Date(now + hours * hour).toISOString()
  it('reports a next run within the policy limit', () => {
    expect(backupTiming(last, inHours(1), 24, now)).toEqual({ kind: 'onTime', nextIn: hour })
  })
  it('warns when the next run lands after the limit', () => {
    expect(backupTiming(last, inHours(30), 24, now)).toEqual({ kind: 'late', nextIn: 30 * hour })
  })
  it('reports an unreported or missed next run with the time left', () => {
    expect(backupTiming(last, null, 24, now)).toEqual({
      kind: 'unscheduled',
      remaining: 22 * hour,
    })
    expect(backupTiming(last, inHours(-1), 24, now)).toEqual({
      kind: 'missed',
      dueAgo: hour,
      remaining: 22 * hour,
    })
  })
  it('reports how far an old backup is past the limit', () => {
    expect(backupTiming(new Date(now - 30 * hour).toISOString(), null, 24, now)).toEqual({
      kind: 'overdue',
      overBy: 6 * hour,
    })
    expect(backupTiming(null, inHours(1), 24, now)).toEqual({ kind: 'none' })
  })
})

describe('size trend', () => {
  it('needs two backups and notices variation', () => {
    expect(sizeTrend([{ at: 1, value: 5 }])).toBeNull()
    expect(
      sizeTrend([
        { at: 1, value: 5 },
        { at: 2, value: 5 },
      ]),
    ).toMatchObject({ count: 2, varies: false })
    expect(
      sizeTrend([
        { at: 1, value: 5 },
        { at: 2, value: 7 },
      ])?.varies,
    ).toBe(true)
  })
})
