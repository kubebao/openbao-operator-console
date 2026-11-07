import { Link } from '@tanstack/react-router'
import { Activity, Bell, Boxes, GitBranch, Settings2 } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { initials } from '../../lib/format'
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
          {failedConnections > 0 && (
            <span className="nav-warning" aria-label="Clusters need attention" />
          )}
        </Link>
        <Link to="/activity" activeProps={{ className: 'nav-active' }}>
          <Activity size={18} />
          <span>Activity</span>
        </Link>
        {snapshot?.viewer.admin && (
          <Link to="/settings" activeProps={{ className: 'nav-active' }}>
            <Settings2 size={18} />
            <span>Settings</span>
          </Link>
        )}
      </nav>
      {snapshot && (
        <div className="sidebar-bottom">
          <Profile viewer={snapshot.viewer} live={snapshot.mode === 'live'} />
          <p className="build-version">
            <span>Console {snapshot.build.version}</span>
            {/* A development build uses the short commit as its version; show it once. */}
            {snapshot.build.commit && !snapshot.build.commit.startsWith(snapshot.build.version) && (
              <code title={`Commit ${snapshot.build.commit}`}>
                {snapshot.build.commit.slice(0, 12)}
              </code>
            )}
          </p>
        </div>
      )}
    </aside>
  )
}

function Profile({ viewer, live }: { viewer: Snapshot['viewer']; live: boolean }) {
  return (
    <div className="profile">
      <div className="avatar" aria-hidden="true">
        {initials(viewer)}
      </div>
      <div>
        <strong title={viewer.subject}>{viewer.name || viewer.subject}</strong>
        {live && (
          <form method="post" action="/auth/logout">
            <button className="sign-out" type="submit">
              Sign out
            </button>
          </form>
        )}
      </div>
    </div>
  )
}
