import { useEffect, useRef, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { Navigate, Outlet, useLocation } from '@tanstack/react-router'
import { APIError, useSetup, useSnapshot } from '../api/client'
import type { Snapshot } from '../api/schema'
import { clusterNames } from '../lib/filters'
import { operatorNeedsAttention } from '../lib/tone'
import { SignIn } from './sign-in'
import { Button } from './ui/button'
import { SearchDialog } from './workspace/search-dialog'
import { Sidebar } from './workspace/sidebar'
import { RefreshBanner } from './workspace/refresh-banner'
import { Topbar } from './workspace/topbar'
import { useRouteFocus } from './workspace/use-route-focus'
import type { Theme } from './workspace/use-theme'
import { useTheme } from './workspace/use-theme'
import { WorkspaceContent } from './workspace/workspace-content'

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
  // A failed background refresh keeps the last setup status. Only a failed first load blocks.
  if (setup.data === undefined)
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

// useSearchShortcut toggles the search dialog with Cmd+K or Ctrl+K.
function useSearchShortcut(setOpen: Dispatch<SetStateAction<boolean>>) {
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === 'k') {
        event.preventDefault()
        setOpen((open) => !open)
      }
    }
    window.addEventListener('keydown', listener)
    return () => {
      window.removeEventListener('keydown', listener)
    }
  }, [setOpen])
}

function findingCount(snapshot: Snapshot | undefined): number {
  return snapshot?.deployments.reduce((total, item) => total + item.issues.length, 0) ?? 0
}

// failedConnectionCount counts clusters whose collection or enrolled operators lack
// current, available evidence.
function failedConnectionCount(snapshot: Snapshot | undefined): number {
  return (
    snapshot?.connections.filter(
      (connection) =>
        connection.collection !== 'current' || connection.operators.some(operatorNeedsAttention),
    ).length ?? 0
  )
}

function WorkspaceShell({ theme, setTheme }: { theme: Theme; setTheme: (theme: Theme) => void }) {
  const setup = useSetup()
  const snapshot = useSnapshot()
  const [searching, setSearching] = useState(false)
  const main = useRef<HTMLElement>(null)
  useSearchShortcut(setSearching)
  useRouteFocus(main)
  if (snapshot.isError && snapshot.error instanceof APIError && snapshot.error.status === 401)
    return <SignIn />
  const failedConnections = failedConnectionCount(snapshot.data)
  const stale = setup.isError || (snapshot.isError && snapshot.data !== undefined)
  return (
    <div className="app-shell">
      <a
        className="skip-link"
        href="#main-content"
        onClick={(event) => {
          event.preventDefault()
          main.current?.focus()
        }}
      >
        Skip to main content
      </a>
      <Sidebar
        snapshot={snapshot.data}
        findings={findingCount(snapshot.data)}
        failedConnections={failedConnections}
      />
      <div className="workspace">
        <Topbar
          snapshot={snapshot.data}
          failedConnections={failedConnections}
          theme={theme}
          onTheme={setTheme}
          onSearch={() => {
            setSearching(true)
          }}
        />
        <main className="main-content" id="main-content" ref={main} tabIndex={-1}>
          {stale && (
            <RefreshBanner
              retrying={setup.isFetching || snapshot.isFetching}
              onRetry={() => {
                if (setup.isError) void setup.refetch()
                if (snapshot.isError) void snapshot.refetch()
              }}
            />
          )}
          <WorkspaceContent snapshot={snapshot} />
        </main>
      </div>
      <SearchDialog
        deployments={snapshot.data?.deployments ?? []}
        clusters={clusterNames(snapshot.data)}
        open={searching}
        onOpenChange={setSearching}
      />
    </div>
  )
}
