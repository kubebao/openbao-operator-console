import { AlertCircle, AlertTriangle, Check, CircleHelp, Info } from 'lucide-react'
import type { Deployment, Issue } from '../api/schema'
import type { CollectionState } from '../lib/format'
import { absolute, age, collectionLabels, healthLabels, until } from '../lib/format'

export function HealthBadge({ health }: { health: Deployment['health'] }) {
  return (
    <span className={`badge status-${health}`}>
      <span className="status-dot" />
      {healthLabels[health]}
    </span>
  )
}

export function SeverityBadge({ severity }: { severity: Issue['severity'] }) {
  const Icon = severity === 'critical' ? AlertCircle : severity === 'high' ? AlertTriangle : Info
  return (
    <span className={`badge severity-${severity}`}>
      <Icon size={13} />
      {severity === 'critical' ? 'Critical' : severity === 'high' ? 'High' : 'Info'}
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
  if (value === null) return <>{missing ?? (scheduled ? 'Not scheduled' : 'Not observed')}</>
  return (
    <time dateTime={value} title={absolute(value)}>
      {scheduled ? until(value) : age(value)}
    </time>
  )
}
