import {
  Check,
  CircleDashed,
  GitCompareArrows,
  Package,
  RotateCcw,
  Workflow,
  X,
} from 'lucide-react'
import type { Deployment } from '../../api/schema'
import { fieldSeverity } from '../../lib/findings'
import { cleanupStates, recoveryPhases, roleLabels } from '../../lib/format'
import { observedGeneration, replicaLag, upgradeLabels, upgradeState } from '../../lib/lifecycle'
import { DetailRows } from '../page'
import { Timestamp } from '../status'
import { SectionTitle } from '../info-tip'

type Recovery = NonNullable<Deployment['recovery']>
type Replica = NonNullable<Deployment['replica']>

const recoverySteps = ['pending', 'validating', 'running', 'completed'] as const

function stepClass(done: boolean, active: boolean): string {
  if (done) return 'done'
  return active ? 'active' : ''
}

function RecoverySteps({ recovery }: { recovery: Recovery }) {
  const failed = recovery.phase === 'failed' || recovery.phase === 'unknown'
  const current = failed ? -1 : recoverySteps.indexOf(recovery.phase)
  return (
    <ol className="phase-steps" aria-label={`Recovery phase: ${recoveryPhases[recovery.phase]}`}>
      {recoverySteps.map((step, index) => {
        const done = !failed && (index < current || recovery.phase === 'completed')
        const active = !failed && index === current && recovery.phase !== 'completed'
        return (
          <li
            key={step}
            className={stepClass(done, active)}
            aria-current={active ? 'step' : undefined}
          >
            {done ? <Check size={13} /> : <CircleDashed size={13} />}
            {recoveryPhases[step]}
          </li>
        )
      })}
      {failed && (
        <li className="failed" aria-current="step">
          <X size={13} />
          {recoveryPhases[recovery.phase]}
        </li>
      )}
    </ol>
  )
}

function generationLabel(observed: number | null, requested: number): string {
  if (observed === null) return 'No conditions reported'
  if (observed >= requested) return `Generation ${String(observed)} (current)`
  return `Generation ${String(observed)} of ${String(requested)}`
}

function VersionSection({ deployment }: { deployment: Deployment }) {
  const upgrade = upgradeState(deployment)
  const failure = deployment.upgradeFailure
  return (
    <section className="detail-section">
      <SectionTitle
        icon={<Package size={17} />}
        tipLabel="About upgrades"
        tip="Upgrade state comes from the operator's recorded failure, its Upgrading condition, and the requested and running versions. Inspect the operator's post-upgrade checks before retrying a failed upgrade."
      >
        Version
      </SectionTitle>
      <DetailRows
        rows={[
          { label: 'Running version', value: deployment.version || 'Not observed' },
          { label: 'Requested version', value: deployment.desiredVersion || 'Not observed' },
          {
            label: 'Upgrade',
            value: (
              <span className={`upgrade-state upgrade-${upgrade}`}>{upgradeLabels[upgrade]}</span>
            ),
            finding: fieldSeverity(deployment, 'upgrade'),
          },
          ...(failure
            ? [
                {
                  label: 'Failure reason',
                  value: <code>{failure.reason || 'Not reported'}</code>,
                  finding: fieldSeverity(deployment, 'upgrade'),
                },
                { label: 'Failure recorded', value: <Timestamp value={failure.at} /> },
              ]
            : []),
        ]}
      />
    </section>
  )
}

function ReconciliationSection({ deployment }: { deployment: Deployment }) {
  return (
    <section className="detail-section">
      <SectionTitle
        icon={<Workflow size={17} />}
        tipLabel="About reconciliation"
        tip="Conditions carry the generation the operator last acted on. A lower generation means a specification change has not been reconciled yet."
      >
        Reconciliation
      </SectionTitle>
      <DetailRows
        rows={[
          { label: 'Resource generation', value: deployment.generation },
          {
            label: 'Reported by operator',
            value: generationLabel(observedGeneration(deployment), deployment.generation),
          },
          { label: 'Operator version label', value: deployment.operatorVersion ?? 'Not observed' },
        ]}
      />
    </section>
  )
}

function RecoverySection({ deployment }: { deployment: Deployment }) {
  const recovery = deployment.recovery
  const failed = recovery?.phase === 'failed' || recovery?.phase === 'unknown'
  return (
    <section className="detail-section">
      <SectionTitle
        icon={<RotateCcw size={17} />}
        tipLabel="About recovery evidence"
        tip="Historical completion does not establish current client availability. Unknown execution requires investigation."
      >
        Recovery
      </SectionTitle>
      {recovery ? (
        <>
          <RecoverySteps recovery={recovery} />
          <DetailRows
            rows={[
              { label: 'Request', value: recovery.requestName },
              {
                label: 'Mechanism',
                value: recovery.mechanism === 'snapshot' ? 'Snapshot' : 'Native',
              },
              {
                label: 'Phase',
                value: recoveryPhases[recovery.phase],
                finding: failed ? fieldSeverity(deployment, 'recovery') : null,
              },
              { label: 'Execution stage', value: recovery.stage || 'Not reported' },
              { label: 'Started', value: <Timestamp value={recovery.startedAt} /> },
              {
                label: 'Target application confirmed',
                value: <Timestamp value={recovery.applicationConfirmedAt} />,
              },
              { label: 'Completed', value: <Timestamp value={recovery.completedAt} /> },
              {
                label: 'Cleanup',
                value: cleanupStates[recovery.cleanup],
                finding:
                  recovery.cleanup === 'failed' ? fieldSeverity(deployment, 'recovery') : null,
              },
            ]}
          />
        </>
      ) : (
        <p className="muted">
          No recovery request is recorded for this deployment. Recovery runs through the operator
          and your runbooks.
        </p>
      )}
    </section>
  )
}

function lagLabel(lag: bigint | null): string {
  if (lag === null) return 'Not reported'
  return `${lag.toLocaleString()} ${lag === 1n ? 'entry' : 'entries'}`
}

export function ReplicationSection({
  deployment,
  replica,
}: {
  deployment: Deployment
  replica: Replica
}) {
  return (
    <section className="detail-section">
      <SectionTitle
        icon={<GitCompareArrows size={17} />}
        tipLabel="About replication evidence"
        tip="An applied index does not authorize survivor selection during recovery."
      >
        Replication
      </SectionTitle>
      <DetailRows
        rows={[
          { label: 'Role', value: roleLabels[deployment.role] },
          { label: 'Behind source', value: lagLabel(replicaLag(deployment)) },
          { label: 'Local index', value: <code>{replica.localIndex ?? 'Not reported'}</code> },
          { label: 'Source index', value: <code>{replica.sourceIndex ?? 'Not reported'}</code> },
          { label: 'Observed', value: <Timestamp value={replica.observedAt} /> },
          {
            label: 'History',
            value: replica.historyConflict ? 'Conflict recorded' : 'No conflict recorded',
            finding: fieldSeverity(deployment, 'replicaHistory'),
          },
        ]}
      />
    </section>
  )
}

export function LifecycleTab({ deployment }: { deployment: Deployment }) {
  return (
    <div className="lifecycle-grid">
      <VersionSection deployment={deployment} />
      <ReconciliationSection deployment={deployment} />
      <RecoverySection deployment={deployment} />
      {deployment.replica && (
        <ReplicationSection deployment={deployment} replica={deployment.replica} />
      )}
    </div>
  )
}
