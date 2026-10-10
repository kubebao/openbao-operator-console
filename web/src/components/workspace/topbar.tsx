import { Link } from '@tanstack/react-router'
import { Moon, Search, Sun } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { age } from '../../lib/format'
import { Button } from '../ui/button'
import type { Theme } from './use-theme'

function collectionTitle(failedConnections: number): string {
  if (failedConnections === 0) return 'All enrolled clusters collected'
  const clusters = failedConnections === 1 ? 'cluster' : 'clusters'
  return `${String(failedConnections)} ${clusters} without current evidence`
}

export function Topbar({
  snapshot,
  failedConnections,
  theme,
  onTheme,
  onSearch,
}: {
  snapshot: Snapshot | undefined
  failedConnections: number
  theme: Theme
  onTheme: (theme: Theme) => void
  onSearch: () => void
}) {
  const next = theme === 'light' ? 'dark' : 'light'
  return (
    <header className="topbar">
      <button className="global-search" onClick={onSearch}>
        <Search size={16} />
        <span>Search deployments, clusters, and teams…</span>
        <kbd>⌘ K</kbd>
      </button>
      <div className="topbar-right">
        <Link
          to="/clusters"
          className="collection-summary"
          title={collectionTitle(failedConnections)}
        >
          <span className={`status-dot ${failedConnections > 0 ? 'dot-warning' : ''}`} />
          <span>
            Snapshot refreshed
            <strong>{snapshot ? age(snapshot.collectedAt) : 'Connecting…'}</strong>
          </span>
        </Link>
        <Button
          size="icon"
          variant="ghost"
          aria-label={`Switch to ${next} mode`}
          onClick={() => {
            onTheme(next)
          }}
        >
          {theme === 'light' ? <Moon size={18} /> : <Sun size={18} />}
        </Button>
      </div>
    </header>
  )
}
