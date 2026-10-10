import { useState } from 'react'
import { Check, Copy, Database, ShieldCheck } from 'lucide-react'
import type { Deployment } from '../../api/schema'
import { fieldSeverity } from '../../lib/findings'
import { bytes, restoreOutcomes } from '../../lib/format'
import type { Observation } from '../../lib/history'
import { backupEvents } from '../../lib/history'
import { DetailRows, EmptyState } from '../page'
import { restoreTone } from '../../lib/tone'
import { StatusBadge, Timestamp } from '../status'
import { Button } from '../ui/button'
import { AgeMeter, BackupStrip, Sparkline } from './charts'
import { SectionTitle } from '../info-tip'

function Digest({ value }: { value: string }) {
  const [copied, setCopied] = useState(false)
  const short = value.length > 28 ? `${value.slice(0, 19)}…${value.slice(-8)}` : value
  return (
    <span className="digest-value">
      <code title={value}>{short}</code>
      <Button
        type="button"
        size="icon"
        variant="ghost"
        aria-label="Copy digest"
        onClick={() => {
          void navigator.clipboard.writeText(value).then(() => {
            setCopied(true)
            window.setTimeout(() => {
              setCopied(false)
            }, 2000)
          })
        }}
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </Button>
    </span>
  )
}

export function BackupsTab({
  deployment,
  maxBackupAgeHours,
  observations,
  now,
}: {
  deployment: Deployment
  maxBackupAgeHours: number
  observations: readonly Observation[] | null
  // The snapshot collection time; ages and the activity window are measured from it.
  now: number
}) {
  const backup = deployment.backup
  if (!backup)
    return (
      <EmptyState title="No backup evidence" icon={Database}>
        Backups are not configured for this deployment, or its role does not report them.
      </EmptyState>
    )
  const windowStart = now - 7 * 24 * 60 * 60 * 1000
  const events = observations ? backupEvents(observations) : []
  const sizes = events
    .filter((event) => event.kind === 'success' && event.sizeBytes !== null)
    .map((event) => ({ at: Date.parse(event.at), value: event.sizeBytes ?? 0 }))
  const age = backup.lastSuccessAt ? (now - Date.parse(backup.lastSuccessAt)) / 3_600_000 : null
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
          {age !== null && (
            <>
              <AgeMeter hours={age} limitHours={maxBackupAgeHours} />
              <small>
                {Math.round(age)}h of {maxBackupAgeHours}h allowed by policy
              </small>
            </>
          )}
        </div>
        <div className="summary-stat">
          <span>Next scheduled</span>
          <strong>
            <Timestamp value={backup.nextAt} scheduled missing="Not reported" />
          </strong>
        </div>
        <div className={`summary-stat${failureFinding ? ` finding-${failureFinding}` : ''}`}>
          <span>Consecutive failures</span>
          <strong>{backup.consecutiveFailures}</strong>
          {backup.consecutiveFailures > 0 && <small>Since the last successful backup</small>}
        </div>
      </div>

      <section className="detail-section">
        <SectionTitle
          tipLabel="About backup activity"
          tip="Derived from recorded changes. Several failures between two observations appear as one marker with a count."
        >
          Backup activity, last 7 days
        </SectionTitle>
        {observations === null ? (
          <p className="muted">Loading recorded changes…</p>
        ) : (
          <BackupStrip events={events} start={windowStart} end={now} />
        )}
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
          <DetailRows
            rows={[
              { label: 'Object key', value: <code>{backup.key ?? 'Not reported'}</code> },
              { label: 'Size', value: bytes(backup.sizeBytes) },
              {
                label: 'Digest',
                value: backup.digest ? <Digest value={backup.digest} /> : 'Not reported',
              },
            ]}
          />
          {sizes.length >= 2 && (
            <div className="trend">
              <h3>Size across retained backups</h3>
              <Sparkline points={sizes} format={bytes} label="Backup size" />
            </div>
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
