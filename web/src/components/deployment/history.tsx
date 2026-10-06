import { AlertCircle } from 'lucide-react'
import { useHistory } from '../../api/client'
import { collectionLabels, healthLabels } from '../../lib/format'
import type { HistoryEntry } from '../../lib/history'
import { describeHistory, stateBand } from '../../lib/history'
import { EmptyState } from '../page'
import { Timestamp } from '../status'
import { StateBand } from './charts'

const dayFormat = new Intl.DateTimeFormat(undefined, {
  weekday: 'long',
  month: 'long',
  day: 'numeric',
})
const timeFormat = new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' })

function dayLabel(at: string, now: number): string {
  const date = new Date(at)
  const today = new Date(now)
  today.setHours(0, 0, 0, 0)
  const day = new Date(date)
  day.setHours(0, 0, 0, 0)
  const difference = Math.round((today.getTime() - day.getTime()) / 86_400_000)
  if (difference === 0) return 'Today'
  if (difference === 1) return 'Yesterday'
  return dayFormat.format(date)
}

function groupByDay(entries: readonly HistoryEntry[], now: number) {
  const groups: { label: string; entries: HistoryEntry[] }[] = []
  for (const entry of entries) {
    const label = dayLabel(entry.at, now)
    const last = groups.at(-1)
    if (last?.label === label) last.entries.push(entry)
    else groups.push({ label, entries: [entry] })
  }
  return groups
}

export function HistoryTab({ id, now }: { id: string; now: number }) {
  const history = useHistory(id)
  if (history.isPending) return <p className="muted">Loading recorded changes…</p>
  if (history.isError)
    return (
      <EmptyState title="History unavailable" icon={AlertCircle}>
        The console cannot read recorded observations.
      </EmptyState>
    )
  const observations = history.data.observations
  const windowEnd = now
  const windowStart = windowEnd - 7 * 24 * 60 * 60 * 1000
  const entries = describeHistory(observations)
  const oldest = entries.at(-1)
  return (
    <>
      <section className="detail-section">
        <h2>State over the last 7 days</h2>
        <StateBand
          segments={stateBand(observations, windowStart, windowEnd)}
          start={windowStart}
          end={windowEnd}
        />
        <p className="section-footnote">
          A state lasts until the next recorded change. The console records a change when the
          resource, its state, its collection result, or its findings change. Time while the console
          itself was not running cannot be distinguished from an unchanged state.
        </p>
      </section>
      <section className="detail-section">
        <div className="section-heading">
          <h2>Recorded changes</h2>
          <span className="muted">
            {entries.length} {entries.length === 1 ? 'change' : 'changes'}
            {oldest && (
              <>
                {' '}
                since <Timestamp value={oldest.at} />
              </>
            )}
          </span>
        </div>
        {entries.length === 0 ? (
          <p className="muted">No observations are retained for this deployment.</p>
        ) : (
          <div className="timeline">
            {groupByDay(entries, windowEnd).map((group) => (
              <section key={group.label} className="timeline-day">
                <h3>{group.label}</h3>
                <ol>
                  {group.entries.map((entry) => (
                    <li key={entry.at} className="timeline-entry">
                      <time dateTime={entry.at} className="timeline-time">
                        {timeFormat.format(new Date(entry.at))}
                      </time>
                      <span
                        className={`tone-dot tone-${entry.changes[0]?.tone ?? 'neutral'}`}
                        aria-hidden="true"
                      />
                      <div className="timeline-body">
                        <ul>
                          {entry.changes.map((change) => (
                            <li key={change.title} className={`change tone-${change.tone}`}>
                              <strong>{change.title}</strong>
                              {change.detail && <span>{change.detail}</span>}
                            </li>
                          ))}
                        </ul>
                        <details className="raw-observation">
                          <summary>Recorded snapshot</summary>
                          <dl>
                            <div>
                              <dt>State</dt>
                              <dd>{healthLabels[entry.observation.deployment.health]}</dd>
                            </div>
                            <div>
                              <dt>Collection</dt>
                              <dd>
                                {collectionLabels[entry.observation.deployment.evidence.collection]}
                              </dd>
                            </div>
                            <div>
                              <dt>Generation</dt>
                              <dd>{entry.observation.deployment.generation}</dd>
                            </div>
                            <div>
                              <dt>Resource version</dt>
                              <dd>
                                <code>{entry.observation.deployment.resourceVersion}</code>
                              </dd>
                            </div>
                          </dl>
                        </details>
                      </div>
                    </li>
                  ))}
                </ol>
              </section>
            ))}
          </div>
        )}
        <p className="section-footnote">
          Up to 100 recorded changes are kept for seven days. Missing periods are not proof of an
          unchanged state.
        </p>
      </section>
    </>
  )
}
