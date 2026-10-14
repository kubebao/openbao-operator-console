import type { ReactNode } from 'react'
import { AlertCircle, AlertTriangle, Check, CircleHelp, Info } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { Deployment, Issue, Operator } from '../api/schema'
import type { CollectionState } from '../lib/format'
import type { Tone } from '../lib/tone'
import { operatorTone } from '../lib/tone'
import {
  absolute,
  age,
  collectionLabels,
  healthLabels,
  operatorCheckLabels,
  operatorHealthLabels,
  operatorHealthText,
  until,
} from '../lib/format'

export function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <span className={`badge status-${tone}`}>
      <span className="status-dot" />
      {children}
    </span>
  )
}

export function HealthBadge({ health }: { health: Deployment['health'] }) {
  return <StatusBadge tone={health}>{healthLabels[health]}</StatusBadge>
}

// Operator health is current only while its controller Deployment is collected. The server
// keeps the last reported health when collection fails, so it is not shown as current then.
export function OperatorHealthBadge({
  operator,
}: {
  operator: Pick<Operator, 'health' | 'collection'>
}) {
  if (operator.collection !== 'current')
    return <StatusBadge tone="unknown">{operatorHealthText(operator)}</StatusBadge>
  return (
    <StatusBadge tone={operatorTone[operator.health]}>
      {operatorHealthLabels[operator.health]}
    </StatusBadge>
  )
}

// OperatorCollectionBadge reports whether the enrolled controller Deployment was collected.
export function OperatorCollectionBadge({ collection }: { collection: Operator['collection'] }) {
  if (collection === 'notFound' || collection === 'invalid')
    return <span className="evidence evidence-unsupported">{operatorCheckLabels[collection]}</span>
  return <EvidenceBadge collection={collection} />
}

const severities: Record<Issue['severity'], { icon: LucideIcon; label: string }> = {
  critical: { icon: AlertCircle, label: 'Critical' },
  high: { icon: AlertTriangle, label: 'High' },
  info: { icon: Info, label: 'Info' },
}

export function SeverityBadge({ severity }: { severity: Issue['severity'] }) {
  const { icon: Icon, label } = severities[severity]
  return (
    <span className={`badge severity-${severity}`}>
      <Icon size={13} />
      {label}
    </span>
  )
}

export function EvidenceBadge({ collection }: { collection: CollectionState }) {
  return (
    <span className={`evidence evidence-${collection}`}>
      {collection === 'current' ? <Check size={12} /> : <CircleHelp size={12} />}
      {collectionLabels[collection]}
    </span>
  )
}

function missingTime(scheduled: boolean, missing: string | undefined): string {
  return missing ?? (scheduled ? 'Not scheduled' : 'Not observed')
}

// Relative time with the absolute timestamp available on hover and to assistive technology.
export function Timestamp({
  value,
  scheduled = false,
  missing,
}: {
  value: string | null
  scheduled?: boolean
  missing?: string
}) {
  if (value === null) return <>{missingTime(scheduled, missing)}</>
  return (
    <time dateTime={value} title={absolute(value)}>
      {scheduled ? until(value) : age(value)}
    </time>
  )
}
