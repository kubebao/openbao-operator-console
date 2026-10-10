import { Link } from '@tanstack/react-router'
import { Activity, Bell, Boxes, Cable, GitBranch, Settings2 } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { Brand } from '../brand'

export function Sidebar({
  snapshot,
  findings,
  failedConnections,
}: {
  snapshot: Snapshot | undefined
  findings: number
  failedConnections: number
}) {
  return (
    <aside className="sidebar">
      <Link to="/" className="brand" aria-label="OpenBao Operator Console home">
        <Brand />
      </Link>
      <nav aria-label="Main navigation">
        <Link to="/" activeOptions={{ exact: true }} activeProps={{ className: 'nav-active' }}>
          <Bell size={18} />
          <span>Attention</span>
          {findings > 0 && <span className="nav-count">{findings}</span>}
        </Link>
        <Link to="/fleet" activeProps={{ className: 'nav-active' }}>
          <Boxes size={18} />
          <span>Fleet</span>
        </Link>
        <Link to="/topology" activeProps={{ className: 'nav-active' }}>
          <GitBranch size={18} />
          <span>Topology</span>
        </Link>
        <Link to="/activity" activeProps={{ className: 'nav-active' }}>
          <Activity size={18} />
          <span>Activity</span>
        </Link>
        <Link to="/clusters" activeProps={{ className: 'nav-active' }}>
          <Cable size={18} />
          <span>Clusters</span>
          {failedConnections > 0 && (
            <span className="nav-warning" aria-label="Clusters need attention" />
          )}
        </Link>
        {snapshot?.viewer.admin && (
          <Link to="/settings" activeProps={{ className: 'nav-active' }}>
            <Settings2 size={18} />
            <span>Settings</span>
          </Link>
        )}
      </nav>
      <div className="sidebar-bottom">
        <div className="profile">
          <div className="avatar">PT</div>
          <div>
            <strong>{snapshot?.viewer.name ?? 'Platform team'}</strong>
            {snapshot?.mode === 'live' && (
              <form method="post" action="/auth/logout">
                <button className="sign-out" type="submit">
                  Sign out
                </button>
              </form>
            )}
          </div>
        </div>
      </div>
    </aside>
  )
}
