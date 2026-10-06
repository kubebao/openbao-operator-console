import type { ComponentType, ReactNode } from 'react'
import { AlertCircle, CheckCircle2, Inbox } from 'lucide-react'
import type { Issue } from '../api/schema'

export function PageHeading({
  title,
  description,
  status,
  actions,
}: {
  title: string
  description?: string
  status?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="page-heading">
      <div>
        <div className="page-title">
          <h1>{title}</h1>
          {status}
        </div>
        {description && <p>{description}</p>}
      </div>
      {actions && <div className="heading-actions">{actions}</div>}
    </header>
  )
}

export function EmptyState({
  title,
  icon: Icon = Inbox,
  children,
}: {
  title: string
  icon?: ComponentType<{ size?: number }>
  children: ReactNode
}) {
  return (
    <div className="empty-state">
      <Icon size={26} />
      <h2>{title}</h2>
      <p>{children}</p>
    </div>
  )
}

export interface DetailRow {
  label: string
  value: ReactNode
  // A finding that this row is evidence for. The row is highlighted with its severity.
  finding?: Issue['severity'] | null
}

export function DetailRows({ rows }: { rows: readonly DetailRow[] }) {
  return (
    <dl className="detail-rows">
      {rows.map((row) => (
        <div key={row.label} className={row.finding ? `finding-${row.finding}` : undefined}>
          <dt>{row.label}</dt>
          <dd>{row.value}</dd>
        </div>
      ))}
    </dl>
  )
}

export function Callout({
  tone,
  title,
  children,
}: {
  tone: 'success' | 'error'
  title: string
  children?: ReactNode
}) {
  const Icon = tone === 'success' ? CheckCircle2 : AlertCircle
  return (
    <div className={`callout callout-${tone}`} role="status">
      <Icon size={18} />
      <div>
        <strong>{title}</strong>
        {children && <p>{children}</p>}
      </div>
    </div>
  )
}
