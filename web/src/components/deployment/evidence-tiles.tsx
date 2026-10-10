import type { ReactNode } from 'react'
import { AlertCircle, AlertTriangle, Database, HeartPulse, ShieldCheck, Wrench } from 'lucide-react'
import type { Deployment, Issue } from '../../api/schema'
import type { DeploymentTab, EvidenceDimension } from '../../lib/findings'
import { dimensionSeverity, tabLabels } from '../../lib/findings'
import { restoreOutcomes } from '../../lib/format'
import { observedGeneration, upgradeLabels, upgradeState } from '../../lib/lifecycle'
import { Timestamp } from '../status'
import { AgeMeter, ReplicaDots } from './charts'

type Backup = NonNullable<Deployment['backup']>

function TileStatus({ severity }: { severity: Issue['severity'] | null }) {
  if (severity === 'critical')
    return <AlertCircle size={15} className="tile-status" aria-label="Critical finding" />
  if (severity) return <AlertTriangle size={15} className="tile-status" aria-label="Finding" />
  return null
}

function EvidenceTile({
  label,
  icon,
  dimension,
  deployment,
  tab,
  onSelect,
  children,
  footer,
}: {
  label: string
  icon: ReactNode
  dimension: EvidenceDimension
  deployment: Deployment
  tab: DeploymentTab
  onSelect: (tab: DeploymentTab) => void
  children: ReactNode
  footer?: ReactNode
}) {
  const severity = dimensionSeverity(deployment, dimension)
  return (
    <button
      type="button"
      className={`evidence-tile${severity ? ` tile-${severity}` : ''}`}
      onClick={() => {
        onSelect(tab)
      }}
      aria-label={`${label}: open ${tabLabels[tab]}`}
    >
      <span className="tile-label">
        {icon}
        {label}
        <TileStatus severity={severity} />
      </span>
      <span className="tile-body">{children}</span>
      {footer && <span className="tile-footer">{footer}</span>}
    </button>
  )
}

const sealLabels = { True: 'Sealed', False: 'Unsealed', Unknown: 'Seal state unknown' } as const

function sealedState(deployment: Deployment): string {
  const sealed = deployment.conditions.find((condition) => condition.type === 'OpenBaoSealed')
  return sealed ? sealLabels[sealed.status] : 'Seal state not reported'
}

function backupFooter(backup: Backup): ReactNode {
  const failures = backup.consecutiveFailures
  if (failures > 0)
    return `${String(failures)} consecutive ${failures === 1 ? 'failure' : 'failures'}`
  if (!backup.nextAt) return 'No schedule reported'
  return (
    <>
      Next <Timestamp value={backup.nextAt} scheduled />
    </>
  )
}

function reconciliationFooter(deployment: Deployment): string {
  const generation = observedGeneration(deployment)
  if (generation === null) return 'Reconciliation not reported'
  if (generation >= deployment.generation) return 'Operator caught up with spec'
  return `Operator at generation ${String(generation)} of ${String(deployment.generation)}`
}

interface TileProps {
  deployment: Deployment
  onSelect: (tab: DeploymentTab) => void
}

function AvailabilityTile({ deployment, onSelect }: TileProps) {
  const { readyReplicas: ready, desiredReplicas: desired } = deployment
  return (
    <EvidenceTile
      label="Availability"
      icon={<HeartPulse size={15} />}
      dimension="availability"
      deployment={deployment}
      tab="conditions"
      onSelect={onSelect}
      footer={sealedState(deployment)}
    >
      {ready === null || desired === null ? (
        <span className="muted">Readiness not reported</span>
      ) : (
        <>
          <ReplicaDots ready={ready} desired={desired} />
          <strong>
            {ready} of {desired} ready
          </strong>
        </>
      )}
    </EvidenceTile>
  )
}

// Backup age is measured from the snapshot's collection time, not the browser clock.
function BackupsTile({
  deployment,
  collectedAt,
  limitHours,
  onSelect,
}: TileProps & { collectedAt: string; limitHours: number }) {
  const backup = deployment.backup
  const age = backup?.lastSuccessAt
    ? (Date.parse(collectedAt) - Date.parse(backup.lastSuccessAt)) / 3_600_000
    : null
  return (
    <EvidenceTile
      label="Backups"
      icon={<Database size={15} />}
      dimension="backups"
      deployment={deployment}
      tab="backups"
      onSelect={onSelect}
      footer={backup ? backupFooter(backup) : undefined}
    >
      {backup ? (
        <>
          <strong>
            {backup.lastSuccessAt ? (
              <>
                Completed <Timestamp value={backup.lastSuccessAt} />
              </>
            ) : (
              'No success recorded'
            )}
          </strong>
          {age !== null && <AgeMeter hours={age} limitHours={limitHours} />}
        </>
      ) : (
        <span className="muted">Not configured</span>
      )}
    </EvidenceTile>
  )
}

function RestoreTestTile({ deployment, onSelect }: TileProps) {
  const test = deployment.backup?.restoreTest ?? null
  return (
    <EvidenceTile
      label="Restore test"
      icon={<ShieldCheck size={15} />}
      dimension="restoreTest"
      deployment={deployment}
      tab="backups"
      onSelect={onSelect}
      footer={test ? 'Pinned snapshot, same cluster' : undefined}
    >
      {test ? (
        <strong>
          {restoreOutcomes[test.outcome]}
          {test.finishedAt && (
            <>
              {' '}
              <Timestamp value={test.finishedAt} />
            </>
          )}
        </strong>
      ) : (
        <span className="muted">No result reported</span>
      )}
    </EvidenceTile>
  )
}

function LifecycleTile({ deployment, onSelect }: TileProps) {
  const upgrade = upgradeState(deployment)
  const target =
    upgrade === 'upgrading' && deployment.desiredVersion !== deployment.version
      ? ` → ${deployment.desiredVersion}`
      : ''
  return (
    <EvidenceTile
      label="Lifecycle"
      icon={<Wrench size={15} />}
      dimension="lifecycle"
      deployment={deployment}
      tab="lifecycle"
      onSelect={onSelect}
      footer={reconciliationFooter(deployment)}
    >
      <strong>
        {deployment.version || 'Version not observed'}
        {target}
      </strong>
      <span className={`upgrade-state upgrade-${upgrade}`}>
        Upgrade {upgradeLabels[upgrade].toLocaleLowerCase()}
      </span>
    </EvidenceTile>
  )
}

export function EvidenceTiles({
  deployment,
  collectedAt,
  maxBackupAgeHours,
  onSelect,
}: TileProps & { collectedAt: string; maxBackupAgeHours: number }) {
  return (
    <div className="evidence-tiles">
      <AvailabilityTile deployment={deployment} onSelect={onSelect} />
      <BackupsTile
        deployment={deployment}
        collectedAt={collectedAt}
        limitHours={maxBackupAgeHours}
        onSelect={onSelect}
      />
      <RestoreTestTile deployment={deployment} onSelect={onSelect} />
      <LifecycleTile deployment={deployment} onSelect={onSelect} />
    </div>
  )
}
