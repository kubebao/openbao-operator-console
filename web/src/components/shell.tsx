import { useEffect, useState } from 'react'
import { Link, Navigate, Outlet, useLocation, useNavigate } from '@tanstack/react-router'
import * as Dialog from '@radix-ui/react-dialog'
import {
  Activity,
  Bell,
  Boxes,
  Cable,
  ChevronRight,
  GitBranch,
  Layers3,
  Moon,
  Search,
  Settings2,
  Sun,
  X,
} from 'lucide-react'
import { APIError, useSetup, useSnapshot } from '../api/client'
import { age } from '../lib/format'
import { HealthBadge } from './status'
import { Button } from './ui/button'

type Theme = 'light' | 'dark'
const themeKey = 'console-theme'
const darkQuery = '(prefers-color-scheme: dark)'

function storedTheme(): Theme | null {
  try {
    const stored = localStorage.getItem(themeKey)
    return stored === 'light' || stored === 'dark' ? stored : null
  } catch {
    return null
  }
}

function systemTheme(): Theme {
  return window.matchMedia(darkQuery).matches ? 'dark' : 'light'
}

// The system preference applies until the user chooses a theme explicitly.
function useTheme(): [Theme, (theme: Theme) => void] {
  const [preference, setPreference] = useState(storedTheme)
  const [system, setSystem] = useState(systemTheme)
  useEffect(() => {
    const media = window.matchMedia(darkQuery)
    const listener = () => {
      setSystem(systemTheme())
    }
    media.addEventListener('change', listener)
    return () => {
      media.removeEventListener('change', listener)
    }
  }, [])
  const theme = preference ?? system
  useEffect(() => {
    document.documentElement.dataset['theme'] = theme
  }, [theme])
  return [
    theme,
    (next) => {
      setPreference(next)
      try {
        localStorage.setItem(themeKey, next)
      } catch {
        // The choice still applies for this page view.
      }
    },
  ]
}

export function Shell() {
  const setup = useSetup()
  const { pathname } = useLocation()
  const [theme, setTheme] = useTheme()
  if (setup.isPending)
    return (
      <div className="entry-state" role="status">
        Loading console…
      </div>
    )
  if (setup.isError)
    return (
      <div className="entry-state">
        <h1>Console unavailable</h1>
        <p>Check the connection and try again.</p>
        <Button
          onClick={() => {
            void setup.refetch()
          }}
        >
          Try again
        </Button>
      </div>
    )
  if (setup.data.state !== 'complete' && pathname !== '/setup') return <Navigate to="/setup" />
  if (pathname === '/setup') return <Outlet />
  return <WorkspaceShell theme={theme} setTheme={setTheme} />
}

function WorkspaceShell({ theme, setTheme }: { theme: Theme; setTheme: (theme: Theme) => void }) {
  const snapshot = useSnapshot()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
        event.preventDefault()
        setOpen((value) => !value)
      }
    }
    window.addEventListener('keydown', listener)
    return () => {
      window.removeEventListener('keydown', listener)
    }
  }, [])
  const count =
    snapshot.data?.deployments.reduce((total, deployment) => total + deployment.issues.length, 0) ??
    0
  const results =
    snapshot.data?.deployments
      .filter((deployment) =>
        `${deployment.name} ${deployment.namespace} ${deployment.team} ${deployment.connectionId}`
          .toLocaleLowerCase()
          .includes(query.toLocaleLowerCase()),
      )
      .slice(0, 8) ?? []
  const selected = Math.min(active, Math.max(results.length - 1, 0))
  const openDeployment = (deploymentId: string) => {
    setOpen(false)
    void navigate({ to: '/deployments/$deploymentId', params: { deploymentId } })
  }
  const failedConnections =
    snapshot.data?.connections.filter(
      (connection) =>
        connection.collection !== 'current' ||
        connection.operators.some(
          (operator) => operator.collection !== 'current' || operator.health !== 'available',
        ),
    ).length ?? 0

  if (snapshot.isError && snapshot.error instanceof APIError && snapshot.error.status === 401)
    return (
      <div className="entry-state sign-in-page">
        <div className="entry-brand">
          OpenBao <span>Operator console</span>
        </div>
        <h1>Sign in</h1>
        <p>Use your organization’s identity provider to continue.</p>
        <a
          href={`/auth/login?returnTo=${encodeURIComponent(window.location.pathname + window.location.search)}`}
          className="button button-primary"
        >
          Sign in
        </a>
      </div>
    )

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <Link to="/" className="brand" aria-label="OpenBao console home">
          <svg viewBox="0 0 32 32" aria-hidden="true">
            <path fill="currentColor" d="M16 1 1 16l15 15 15-15z" />
            <path
              fill="none"
              stroke="var(--sidebar)"
              strokeWidth="1"
              d="M16 1v30M1 16h30M5 12h22"
            />
          </svg>
          <span>
            OpenBao<small>Operator console</small>
          </span>
        </Link>
        <nav aria-label="Main navigation">
          <Link to="/" activeOptions={{ exact: true }} activeProps={{ className: 'nav-active' }}>
            <Bell size={18} />
            <span>Attention</span>
            {count > 0 && <span className="nav-count">{count}</span>}
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
          {snapshot.data?.viewer.admin && (
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
              <strong>{snapshot.data?.viewer.name ?? 'Platform team'}</strong>
              {snapshot.data?.mode === 'live' && (
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
      <div className="workspace">
        <header className="topbar">
          <button
            className="global-search"
            onClick={() => {
              setOpen(true)
            }}
          >
            <Search size={16} />
            <span>Search deployments, clusters, and teams…</span>
            <kbd>⌘ K</kbd>
          </button>
          <div className="topbar-right">
            <Link
              to="/clusters"
              className="collection-summary"
              title={
                failedConnections > 0
                  ? `${String(failedConnections)} cluster${failedConnections === 1 ? '' : 's'} without current evidence`
                  : 'All enrolled clusters collected'
              }
            >
              <span className={`status-dot ${failedConnections > 0 ? 'dot-warning' : ''}`} />
              <span>
                Snapshot refreshed
                <strong>{snapshot.data ? age(snapshot.data.collectedAt) : 'Connecting…'}</strong>
              </span>
            </Link>
            <Button
              size="icon"
              variant="ghost"
              aria-label={theme === 'light' ? 'Switch to dark mode' : 'Switch to light mode'}
              onClick={() => {
                setTheme(theme === 'light' ? 'dark' : 'light')
              }}
            >
              {theme === 'light' ? <Moon size={18} /> : <Sun size={18} />}
            </Button>
          </div>
        </header>
        <main className="main-content" id="main-content">
          {snapshot.isPending ? (
            <div className="loading-state">
              <span className="loading-dot" />
              Loading fleet evidence…
            </div>
          ) : snapshot.isError ? (
            <div className="error-state">
              <Layers3 size={30} />
              <h1>Fleet evidence unavailable</h1>
              <p>
                {snapshot.error instanceof APIError && snapshot.error.status === 401
                  ? 'Sign in with your platform identity to view this fleet.'
                  : 'The console cannot refresh evidence. Existing workloads continue independently.'}
              </p>
              {snapshot.error instanceof APIError && snapshot.error.status === 401 ? (
                <a
                  href={`/auth/login?returnTo=${encodeURIComponent(window.location.pathname + window.location.search)}`}
                  className="button button-primary"
                >
                  Sign in
                </a>
              ) : (
                <Button
                  onClick={() => {
                    void snapshot.refetch()
                  }}
                >
                  Try again
                </Button>
              )}
            </div>
          ) : (
            <>
              {snapshot.data.mode === 'demo' && (
                <div className="workspace-notice">
                  <span>DEMONSTRATION</span>Illustrative evidence · no live clusters connected
                </div>
              )}
              <Outlet />
            </>
          )}
        </main>
      </div>
      <Dialog.Root open={open} onOpenChange={setOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="dialog-overlay" />
          <Dialog.Content className="search-dialog">
            <Dialog.Title className="sr-only">Search fleet</Dialog.Title>
            <Dialog.Description className="sr-only">
              Find deployments by name, team, or Kubernetes cluster.
            </Dialog.Description>
            <div className="command-input">
              <Search size={18} />
              <input
                autoFocus
                placeholder="Search your fleet…"
                value={query}
                role="combobox"
                aria-expanded={results.length > 0}
                aria-controls="search-results"
                aria-activedescendant={
                  results[selected] ? `search-result-${results[selected].id}` : undefined
                }
                onChange={(event) => {
                  setQuery(event.target.value)
                  setActive(0)
                }}
                onKeyDown={(event) => {
                  if (event.key === 'ArrowDown') {
                    event.preventDefault()
                    setActive(Math.min(selected + 1, results.length - 1))
                  } else if (event.key === 'ArrowUp') {
                    event.preventDefault()
                    setActive(Math.max(selected - 1, 0))
                  } else if (event.key === 'Enter') {
                    const result = results[selected]
                    if (result) {
                      event.preventDefault()
                      openDeployment(result.id)
                    }
                  }
                }}
                aria-label="Search your fleet"
              />
              <Dialog.Close asChild>
                <Button size="icon" variant="ghost" aria-label="Close search">
                  <X size={17} />
                </Button>
              </Dialog.Close>
            </div>
            <div className="command-results">
              <div className="nav-label" id="search-results-label">
                DEPLOYMENTS
              </div>
              {results.length === 0 && <p className="muted">No deployments match this search.</p>}
              <div role="listbox" id="search-results" aria-labelledby="search-results-label">
                {results.map((deployment, index) => (
                  <button
                    key={deployment.id}
                    id={`search-result-${deployment.id}`}
                    role="option"
                    aria-selected={index === selected}
                    tabIndex={-1}
                    className={index === selected ? 'active' : ''}
                    onMouseMove={() => {
                      setActive(index)
                    }}
                    onClick={() => {
                      openDeployment(deployment.id)
                    }}
                  >
                    <Boxes size={18} />
                    <span>
                      <strong>{deployment.name}</strong>
                      <small>
                        {deployment.connectionId} / {deployment.namespace}
                      </small>
                    </span>
                    <HealthBadge health={deployment.health} />
                    <ChevronRight size={16} />
                  </button>
                ))}
              </div>
            </div>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    </div>
  )
}
