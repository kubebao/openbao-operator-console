import { Fragment } from 'react'
import type { BackupTiming, DayRow, HourCell, HourState } from '../../lib/backup-activity'
import { backupTiming } from '../../lib/backup-activity'
import type { BandSegment } from '../../lib/history'
import { bandLabels, bandStates } from '../../lib/history'
import { absolute, duration } from '../../lib/format'

const dayFormat = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric' })

// Midnight ticks inside a time window, used under the state band and backup strip.
function dayTicks(start: number, end: number): number[] {
  const ticks: number[] = []
  const day = new Date(start)
  day.setHours(24, 0, 0, 0)
  for (; day.getTime() < end; day.setDate(day.getDate() + 1)) ticks.push(day.getTime())
  return ticks
}

function position(value: number, start: number, end: number): string {
  return `${String(((value - start) / (end - start)) * 100)}%`
}

function TimeAxis({ start, end }: { start: number; end: number }) {
  return (
    <div className="time-axis" aria-hidden="true">
      {dayTicks(start, end).map((tick) => (
        <span key={tick} style={{ left: position(tick, start, end) }}>
          {dayFormat.format(tick)}
        </span>
      ))}
    </div>
  )
}

export function StateBand({
  segments,
  start,
  end,
}: {
  segments: readonly BandSegment[]
  start: number
  end: number
}) {
  const present = new Set(segments.map((segment) => segment.state))
  const incidents = segments.filter((segment) =>
    ['attention', 'critical', 'uncollected'].includes(segment.state),
  )
  return (
    <figure className="state-band">
      <div
        className="state-band-track"
        role="img"
        aria-label={`Recorded state from ${absolute(start)} to ${absolute(end)}: ${segments.map((segment) => bandLabels[segment.state]).join(', then ')}.`}
      >
        {segments.map((segment, index) => (
          <span
            key={segment.start}
            className={`state-segment band-${segment.state}${index === segments.length - 1 ? ' state-last' : ''}`}
            style={{
              left: position(segment.start, start, end),
              width: position(segment.end - segment.start + start, start, end),
            }}
            title={`${bandLabels[segment.state]}: ${absolute(segment.start)} – ${absolute(segment.end)}`}
          />
        ))}
        {incidents.map((segment) => (
          <span
            key={`marker-${String(segment.start)}`}
            className={`state-incident band-${segment.state}`}
            style={{ left: position(segment.start, start, end) }}
            title={`${bandLabels[segment.state]}: ${absolute(segment.start)} – ${absolute(segment.end)}`}
            aria-hidden="true"
          />
        ))}
      </div>
      <TimeAxis start={start} end={end} />
      <figcaption className="chart-legend">
        {bandStates
          .filter((state) => present.has(state))
          .map((state) => (
            <span key={state}>
              <span className={`legend-swatch band-${state}`} />
              {bandLabels[state]}
            </span>
          ))}
      </figcaption>
      {incidents.length > 0 && (
        <details className="chart-incidents">
          <summary>
            {incidents.length} recorded {incidents.length === 1 ? 'incident' : 'incidents'} ·
            markers show occurrence, not duration
          </summary>
          <ul>
            {incidents.map((segment) => (
              <li key={segment.start}>
                {bandLabels[segment.state]}: {absolute(segment.start)} – {absolute(segment.end)} (
                {duration(segment.end - segment.start)})
              </li>
            ))}
          </ul>
        </details>
      )}
    </figure>
  )
}

const hourFormat = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })

const hourStateLabels: Record<HourState, string> = {
  completed: 'Completed',
  failed: 'Failure observed',
  none: 'No backup',
  unrecorded: 'Not recorded',
  unloaded: 'Not loaded',
  future: '',
  outside: '',
}

function cellTitle(cell: HourCell): string {
  const when = `${dayFormat.format(cell.start)}, ${hourFormat.format(cell.start)}–${hourFormat.format(cell.end)}`
  const parts = [
    cell.completed > 0 && `${String(cell.completed)} completed`,
    cell.failures > 0 &&
      `${String(cell.failures)} ${cell.failures === 1 ? 'failure' : 'failures'} observed`,
  ].filter(Boolean)
  return `${when} · ${parts.length > 0 ? parts.join(', ') : hourStateLabels[cell.state]}`
}

// BackupWeek shows one cell per hour for the last seven days, so a regular schedule reads as
// a pattern and a missed run as a gap. Hours before retained history are not recorded.
export function BackupWeek({ rows }: { rows: readonly DayRow[] }) {
  const cells = rows.flatMap((row) => row.cells)
  const completed = cells.reduce((total, cell) => total + cell.completed, 0)
  const failed = cells.reduce((total, cell) => total + cell.failures, 0)
  const present = new Set(cells.map((cell) => cell.state))
  return (
    <figure className="backup-week">
      <div
        className="backup-week-grid"
        role="img"
        aria-label={`${String(completed)} backups completed and ${String(failed)} failures observed in the last seven days.`}
      >
        <span aria-hidden="true" />
        {[0, 6, 12, 18].map((hourOfDay) => (
          <span
            key={hourOfDay}
            className="backup-week-hour"
            aria-hidden="true"
            style={{ gridColumn: `${String(hourOfDay + 2)} / span 6` }}
          >
            {String(hourOfDay).padStart(2, '0')}:00
          </span>
        ))}
        {rows.map((row) => (
          <Fragment key={row.start}>
            <span className="backup-week-day" aria-hidden="true">
              {dayFormat.format(row.start)}
            </span>
            {row.cells.map((cell, hourOfDay) => (
              <span
                key={hourOfDay}
                className={`backup-hour hour-${cell.state}`}
                title={
                  cell.state === 'future' || cell.state === 'outside' ? undefined : cellTitle(cell)
                }
              />
            ))}
          </Fragment>
        ))}
      </div>
      <figcaption className="chart-legend">
        <span>
          <span className="legend-swatch hour-completed" />
          Completed ({completed})
        </span>
        <span>
          <span className="legend-swatch hour-failed" />
          Failure observed ({failed})
        </span>
        <span>
          <span className="legend-swatch hour-none" />
          No backup
        </span>
        {(['unrecorded', 'unloaded'] as const)
          .filter((state) => present.has(state))
          .map((state) => (
            <span key={state}>
              <span className={`legend-swatch hour-${state}`} />
              {hourStateLabels[state]}
            </span>
          ))}
      </figcaption>
    </figure>
  )
}

const timingTone: Record<BackupTiming['kind'], 'within' | 'near' | 'over'> = {
  none: 'over',
  overdue: 'over',
  unscheduled: 'near',
  late: 'near',
  missed: 'near',
  onTime: 'within',
}

function timingText(timing: BackupTiming, limitHours: number): string {
  const limit = `${String(limitHours)}h policy limit`
  switch (timing.kind) {
    case 'none':
      return 'No successful backup is recorded.'
    case 'overdue':
      return `${duration(timing.overBy)} past the ${limit}.`
    case 'unscheduled':
      return `No next run reported. ${duration(timing.remaining)} until the ${limit}.`
    case 'missed':
      return `The next run was due ${duration(timing.dueAgo)} ago. ${duration(timing.remaining)} until the ${limit}.`
    case 'late':
      return `The next run in ${duration(timing.nextIn)} is after the ${limit}.`
    case 'onTime':
      return `Next run in ${duration(timing.nextIn)}, within the ${limit}.`
  }
}

// BackupTimeline runs from the last successful backup to the policy limit. The fill marks
// the backup's age and the marker the next reported run.
export function BackupTimeline({
  lastSuccessAt,
  nextAt,
  limitHours,
  now,
}: {
  lastSuccessAt: string | null
  nextAt: string | null
  limitHours: number
  now: number
}) {
  const timing = backupTiming(lastSuccessAt, nextAt, limitHours, now)
  if (lastSuccessAt === null) return <small>{timingText(timing, limitHours)}</small>
  const last = Date.parse(lastSuccessAt)
  const span = limitHours * 60 * 60 * 1000
  const at = (time: number) => `${String(Math.min(Math.max((time - last) / span, 0), 1) * 100)}%`
  const next = nextAt === null ? null : Date.parse(nextAt)
  // The fill follows the backup's age, as the attention rule does; the text adds the outlook.
  const fill = meterTone((now - last) / span)
  return (
    <div className={`backup-timeline timeline-${timingTone[timing.kind]} fill-${fill}`}>
      <div
        className="backup-timeline-track"
        role="meter"
        aria-valuemin={0}
        aria-valuemax={limitHours}
        aria-valuenow={Math.min((now - last) / 3_600_000, limitHours)}
        aria-label={`Backup age against the ${String(limitHours)} hour policy limit`}
      >
        <span className="backup-timeline-fill" style={{ width: at(now) }} />
        {next !== null && next >= now && (
          <span className="backup-timeline-next" style={{ left: at(next) }} />
        )}
      </div>
      <div className="backup-timeline-scale" aria-hidden="true">
        <span>Last backup</span>
        <span>{limitHours}h limit</span>
      </div>
      <small>{timingText(timing, limitHours)}</small>
    </div>
  )
}

// Inline trend for a single series; the surrounding text names the values.
export function Sparkline({
  points,
  label,
}: {
  points: readonly { at: number; value: number }[]
  label: string
}) {
  const first = points[0]
  const last = points.at(-1)
  if (!first || !last || points.length < 2) return null
  const width = 120
  const height = 24
  const pad = 3
  const values = points.map((point) => point.value)
  const min = Math.min(...values)
  const max = Math.max(...values)
  const x = (at: number) => pad + ((at - first.at) / (last.at - first.at || 1)) * (width - pad * 2)
  const y = (value: number) =>
    height - pad - ((value - min) / (max - min || 1)) * (height - pad * 2)
  const path = points
    .map((point, index) => `${index ? 'L' : 'M'}${String(x(point.at))},${String(y(point.value))}`)
    .join(' ')
  return (
    <svg
      className="sparkline"
      viewBox={`0 0 ${String(width)} ${String(height)}`}
      role="img"
      aria-label={label}
    >
      <path d={path} className="sparkline-line" />
      <circle cx={x(last.at)} cy={y(last.value)} r={2.5} className="sparkline-dot" />
    </svg>
  )
}

function meterTone(ratio: number): 'over' | 'near' | 'within' {
  if (ratio > 1) return 'over'
  return ratio > 0.75 ? 'near' : 'within'
}

// Age against a threshold. The fill turns amber near the threshold and red past it.
export function AgeMeter({ hours, limitHours }: { hours: number; limitHours: number }) {
  const ratio = hours / limitHours
  const tone = meterTone(ratio)
  return (
    <div
      className={`age-meter meter-${tone}`}
      role="meter"
      aria-valuemin={0}
      aria-valuemax={limitHours}
      aria-valuenow={Math.min(hours, limitHours)}
      aria-label={`Backup age ${String(Math.round(hours))} of ${String(limitHours)} hours allowed`}
    >
      <span style={{ width: `${String(Math.min(ratio, 1) * 100)}%` }} />
    </div>
  )
}

export function ReplicaDots({ ready, desired }: { ready: number | null; desired: number | null }) {
  if (desired === null || desired > 12) return null
  return (
    <span className="replica-dots" aria-hidden="true">
      {Array.from({ length: desired }, (_, index) => (
        <span key={index} className={ready !== null && index < ready ? 'ready' : ''} />
      ))}
    </span>
  )
}
