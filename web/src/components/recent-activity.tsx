import { AlertCircle, Info, ShieldCheck } from 'lucide-react'
import type { Activity } from '../api/activity'
import type { ChangeTone } from '../lib/changes'
import { describeActivity } from '../lib/activity'
import { Timestamp } from './status'

const icons: Record<ChangeTone, typeof AlertCircle> = {
  critical: AlertCircle,
  warning: AlertCircle,
  success: ShieldCheck,
  neutral: Info,
}

export function RecentActivity({ events }: { events: readonly Activity[] }) {
  return (
    <ul className="activity-list">
      {events.map((event) => {
        const change = describeActivity(event)
        const Icon = icons[change.tone]
        const resource = event.deploymentName
          ? `${event.namespace}/${event.deploymentName}`
          : `${event.namespace} · resource ID ${event.deploymentId}`
        return (
          <li key={event.id}>
            <span className={`activity-symbol activity-${change.tone}`}>
              <Icon size={17} />
            </span>
            <div>
              <strong>{change.title}</strong>
              <small>{change.detail ? `${resource} · ${change.detail}` : resource}</small>
            </div>
            <Timestamp value={event.recordedAt} />
          </li>
        )
      })}
    </ul>
  )
}
