import { AlertCircle, ShieldCheck } from 'lucide-react'
import type { Snapshot } from '../api/schema'
import { Timestamp } from './status'

export function RecentActivity({ snapshot, limit = 5 }: { snapshot: Snapshot; limit?: number }) {
  return (
    <ul className="activity-list">
      {snapshot.activity.slice(0, limit).map((event) => (
        <li key={event.id}>
          <span className={`activity-symbol activity-${event.outcome}`}>
            {event.outcome === 'success' ? <ShieldCheck size={17} /> : <AlertCircle size={17} />}
          </span>
          <div>
            <strong>{event.title}</strong>
            <small>{event.detail}</small>
          </div>
          <Timestamp value={event.at} />
        </li>
      ))}
    </ul>
  )
}
