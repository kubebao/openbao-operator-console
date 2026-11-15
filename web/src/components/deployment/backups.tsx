import { useState } from 'react'
import { Check, Copy, Database, ShieldCheck } from 'lucide-react'
import { useCharts } from '../../api/client'
import type { ChartHistory, Deployment } from '../../api/schema'
import { fieldSeverity } from '../../lib/findings'
import { sizeTrend } from '../../lib/backup-activity'
import { backupWeek } from '../../lib/backup-activity'
import { bytes, restoreOutcomes } from '../../lib/format'
import { ChartStatus } from './chart-status'
import { backupEvents } from '../../lib/history'
import { DetailRows, EmptyState } from '../page'
import { restoreTone } from '../../lib/tone'
import { StatusBadge, Timestamp } from '../status'
import { Button } from '../ui/button'
import { BackupTimeline, BackupWeek, Sparkline } from './charts'
import { SectionTitle } from '../info-tip'

type CopyState = 'idle' | 'copied' | 'failed'

const copyMessages: Record<CopyState, string> = {
  idle: '',
  copied: 'Digest copied.',
  failed: 'The digest could not be copied. Select it and copy it manually.',
}

function Digest({ value }: { value: string }) {
  const [state, setState] = useState<CopyState>('idle')
  const short = value.length > 28 ? `${value.slice(0, 19)}…${value.slice(-8)}` : value
  const settle = (next: CopyState) => {
    setState(next)
    window.setTimeout(() => {
      setState('idle')
    }, 4000)
  }
  return (
    <span className="digest-value">
      <code title={value}>{state === 'failed' ? value : short}</code>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        aria-label="Copy digest"
        onClick={() => {
          navigator.clipboard.writeText(value).then(
            () => {
              settle('copied')
            },
            () => {
              settle('failed')
            },
          )
        }}
      >
        {state === 'copied' ? <Check size={14} /> : <Copy size={14} />}
      </Button>
      <span role="status" className={state === 'failed' ? 'digest-error' : 'sr-only'}>
        {copyMessages[state]}
      </span>
    </span>
  )
}

// The oldest retained observation; hours before it are not recorded.
function recordedFrom(data: ChartHistory): number | null {
  const at = data.baseline?.recordedAt ?? data.observations.at(-1)?.recordedAt
  return at ? Date.parse(at) : null
}

function chartEvents(data: ChartHistory | undefined) {
  if (!data) return []
  const observations = [...(data.baseline ? [data.baseline] : []), ...data.observations]
  const start = Date.parse(data.start)
  const end = Date.parse(data.end)
  return backupEvents(observations).filter((event) => {
    const at = Date.parse(event.at)
    return at >= start && at <= end
  })
}

const shortDay = new Intl.DateTimeFormat(undefined, { weekday: 'short', day: 'numeric' })

// BackupSize states the latest size and how retained sizes changed. A line is drawn only
// when the size actually varied.
function BackupSize({
  sizeBytes,
  points,
}: {
  sizeBytes: number | null
  points: readonly { at: number; value: number }[]
}) {
  const trend = sizeTrend(points)
  if (sizeBytes === null) return <>Not observed</>
  if (!trend) return <>{bytes(sizeBytes)}</>
  const change = trend.last.value - trend.first.value
  const percent = trend.first.value ? (change / trend.first.value) * 100 : 0
  return (
    <span className="backup-size">
      <span>
        {bytes(sizeBytes)}
        <small>
          {trend.varies
            ? ` · ${change >= 0 ? '+' : '−'}${bytes(Math.abs(change))} (${Math.abs(percent).toFixed(1)}%) since ${shortDay.format(trend.first.at)}`
            : ` · unchanged across ${String(trend.count)} backups`}
        </small>
      </span>
      {trend.varies && (
        <Sparkline
          points={points}
          label={`Backup size from ${bytes(trend.first.value)} to ${bytes(trend.last.value)} across ${String(trend.count)} backups`}
        />
      )}
    </span>
  )
}

export function BackupsTab({
  deployment,
  maxBackupAgeHours,
  now,
}: {
  deployment: Deployment
  maxBackupAgeHours: number
  // When the snapshot was generated; ages and the activity window are measured from it.
  now: number
}) {
  const backup = deployment.backup
  const charts = useCharts(deployment.id, backup !== null)
  if (!backup)
    return (
      <EmptyState title="No backup evidence" icon={Database}>
        Backups are not configured for this deployment, or its role does not report them.
      </EmptyState>
    )
  const data = charts.data
  const events = chartEvents(data)
  const sizes = events
    .filter((event) => event.kind === 'success' && event.sizeBytes !== null)
    .map((event) => ({ at: Date.parse(event.at), value: event.sizeBytes ?? 0 }))
  const successFinding = fieldSeverity(deployment, 'backupSuccess')
  const failureFinding = fieldSeverity(deployment, 'backupFailures')
  const test = backup.restoreTest
  return (
    <>
      <div className="backup-summary">
        <div className={`summary-stat${successFinding ? ` finding-${successFinding}` : ''}`}>
          <span>Last successful backup</span>
          <strong>
            <Timestamp value={backup.lastSuccessAt} missing="None recorded" />
          </strong>
          <BackupTimeline
            lastSuccessAt={backup.lastSuccessAt}
            nextAt={backup.nextAt}
            limitHours={maxBackupAgeHours}
            now={now}
          />
        </div>
        <div className={`summary-stat${failureFinding ? ` finding-${failureFinding}` : ''}`}>
          <span>Consecutive failures</span>
          <strong>{backup.consecutiveFailures ?? 'Unknown'}</strong>
          {backup.consecutiveFailures !== null && backup.consecutiveFailures > 0 && (
            <small>Since the last successful backup</small>
          )}
        </div>
      </div>

      <section className="detail-section">
        <SectionTitle
          tipLabel="About backup activity"
          tip="One cell per local clock hour, using all retained chart evidence in the displayed seven-day window. A cell can contain several completions. Failures outrank completions and are counted when observed."
        >
          Backup activity, last 7 days
        </SectionTitle>
        {data && (
          <BackupWeek
            rows={backupWeek(
              events,
              Date.parse(data.end),
              recordedFrom(data),
              false,
              Date.parse(data.start),
            )}
          />
        )}
        <ChartStatus query={charts} />
      </section>

      <div className="detail-two-column">
        <section className="detail-section">
          <SectionTitle
            icon={<Database size={17} />}
            tipLabel="About backup evidence"
            tip="Completion age is not a measured recovery point objective. A past upload does not prove the repository is still readable."
          >
            Latest backup
          </SectionTitle>
          {backup.key === null && backup.sizeBytes === null && backup.digest === null ? (
            <p className="muted">No backup object is reported.</p>
          ) : (
            <DetailRows
              rows={[
                { label: 'Object key', value: <code>{backup.key ?? 'Not reported'}</code> },
                {
                  label: 'Size',
                  value: <BackupSize sizeBytes={backup.sizeBytes} points={sizes} />,
                },
                {
                  label: 'Digest',
                  value: backup.digest ? <Digest value={backup.digest} /> : 'Not reported',
                },
              ]}
            />
          )}
        </section>
        <section className="detail-section">
          <SectionTitle
            icon={<ShieldCheck size={17} />}
            tipLabel="About restore tests"
            tip="A pass covers a pinned snapshot restored into a disposable target in the same cluster. Client logins and secret reads are not tested."
          >
            Restore test
          </SectionTitle>
          {test ? (
            <>
              <DetailRows
                rows={[
                  {
                    label: 'Latest outcome',
                    value: (
                      <StatusBadge tone={restoreTone[test.outcome]}>
                        {restoreOutcomes[test.outcome]}
                      </StatusBadge>
                    ),
                    finding: fieldSeverity(deployment, 'restoreTest'),
                  },
                  { label: 'Result recorded', value: <Timestamp value={test.finishedAt} /> },
                  {
                    label: 'Last successful test',
                    value: <Timestamp value={test.lastSuccessAt} />,
                  },
                  { label: 'Reason', value: test.reason || 'Not reported' },
                  { label: 'Tested object key', value: <code>{test.key ?? 'Not reported'}</code> },
                ]}
              />
            </>
          ) : (
            <p className="muted">No restore-test result is reported.</p>
          )}
        </section>
      </div>
    </>
  )
}
