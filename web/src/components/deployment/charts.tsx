import type { BackupEvent, BandSegment, BandState } from '../../lib/history'
import { bandLabels } from '../../lib/history'
import { absolute, bytes } from '../../lib/format'

const dayFormat = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric' })

// Midnight ticks inside a time window, used under the state band and backup strip.
function dayTicks(start: number, end: number): number[] {
  const ticks: number[] = []
  const first = new Date(start)
  first.setHours(24, 0, 0, 0)
  for (let tick = first.getTime(); tick < end; tick += 24 * 60 * 60 * 1000) ticks.push(tick)
  return ticks
}

function position(value: number, start: number, end: number): string {
  return `${String(((value - start) / (end - start)) * 100)}%`
}

export function TimeAxis({ start, end }: { start: number; end: number }) {
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
  return (
    <figure className="state-band">
      <div
        className="state-band-track"
        role="img"
        aria-label={`Recorded state from ${absolute(start)} to ${absolute(end)}: ${segments.map((segment) => bandLabels[segment.state]).join(', then ')}.`}
      >
        {segments.map((segment) => (
          <span
            key={segment.start}
            className={`band-${segment.state}`}
            style={{
              left: position(segment.start, start, end),
              width: position(segment.end - segment.start + start, start, end),
            }}
            title={`${bandLabels[segment.state]}: ${absolute(segment.start)} – ${absolute(segment.end)}`}
          />
        ))}
      </div>
      <TimeAxis start={start} end={end} />
      <figcaption className="chart-legend">
        {(Object.keys(bandLabels) as BandState[])
          .filter((state) => present.has(state))
          .map((state) => (
            <span key={state}>
              <span className={`legend-swatch band-${state}`} />
              {bandLabels[state]}
            </span>
          ))}
      </figcaption>
    </figure>
  )
}

export function BackupStrip({
  events,
  start,
  end,
}: {
  events: readonly BackupEvent[]
  start: number
  end: number
}) {
  const visible = events.filter((event) => {
    const at = Date.parse(event.at)
    return at >= start && at <= end
  })
  const completed = visible.filter((event) => event.kind === 'success').length
  const failed = visible
    .filter((event) => event.kind === 'failure')
    .reduce((total, event) => total + event.count, 0)
  return (
    <figure className="backup-strip">
      <div
        className="backup-strip-track"
        role="img"
        aria-label={`${String(completed)} backups completed and ${String(failed)} failures observed in retained history.`}
      >
        {visible.map((event) => (
          <span
            key={`${event.kind}-${event.at}`}
            className={`backup-marker marker-${event.kind}`}
            style={{ left: position(Date.parse(event.at), start, end) }}
            title={
              event.kind === 'success'
                ? `Backup completed ${absolute(event.at)}${event.sizeBytes === null ? '' : ` · ${bytes(event.sizeBytes)}`}`
                : `${event.count === 1 ? 'Failure' : `${String(event.count)} failures`} observed ${absolute(event.at)}`
            }
          />
        ))}
      </div>
      <TimeAxis start={start} end={end} />
      <figcaption className="chart-legend">
        <span>
          <span className="legend-swatch marker-success" />
          Completed ({completed})
        </span>
        <span>
          <span className="legend-swatch marker-failure" />
          Failure observed ({failed})
        </span>
      </figcaption>
    </figure>
  )
}

// Single-series trend: the caller's heading names the series, so there is no legend.
export function Sparkline({
  points,
  format,
  label,
}: {
  points: readonly { at: number; value: number }[]
  format: (value: number) => string
  label: string
}) {
  const first = points[0]
  const last = points.at(-1)
  if (!first || !last || points.length < 2) return null
  const width = 240
  const height = 48
  const pad = 6
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
    <figure className="sparkline">
      <svg
        viewBox={`0 0 ${String(width)} ${String(height)}`}
        role="img"
        aria-label={`${label}: ${format(first.value)} to ${format(last.value)} across ${String(points.length)} backups.`}
        preserveAspectRatio="none"
      >
        <path d={path} className="sparkline-line" vectorEffect="non-scaling-stroke" />
      </svg>
      <span
        className="sparkline-dot"
        style={{
          left: `${String((x(last.at) / width) * 100)}%`,
          top: `${String((y(last.value) / height) * 100)}%`,
        }}
      />
      <figcaption>
        <span>{format(first.value)}</span>
        <strong>{format(last.value)}</strong>
      </figcaption>
    </figure>
  )
}

// Age against a threshold. The fill turns amber near the threshold and red past it.
export function AgeMeter({ hours, limitHours }: { hours: number; limitHours: number }) {
  const ratio = hours / limitHours
  const tone = ratio > 1 ? 'over' : ratio > 0.75 ? 'near' : 'within'
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
