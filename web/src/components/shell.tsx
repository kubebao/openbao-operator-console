import { useEffect, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { Navigate, Outlet, useLocation } from '@tanstack/react-router'
import { APIError, useSetup, useSnapshot } from '../api/client'
import type { Snapshot } from '../api/schema'
import { SignIn } from './sign-in'
import { Button } from './ui/button'
import { SearchDialog } from './workspace/search-dialog'
import { Sidebar } from './workspace/sidebar'
import { Topbar } from './workspace/topbar'
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

// useSearchShortcut toggles the search dialog with Cmd+K or Ctrl+K.
function useSearchShortcut(setOpen: Dispatch<SetStateAction<boolean>>) {
  useEffect(() => {
    const listener = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key === 'k') {
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
        connection.collection !== 'current' ||
        connection.operators.some(
          (operator) => operator.collection !== 'current' || operator.health !== 'available',
        ),
    ).length ?? 0
  )
}

function WorkspaceShell({ theme, setTheme }: { theme: Theme; setTheme: (theme: Theme) => void }) {
  const snapshot = useSnapshot()
  const [searching, setSearching] = useState(false)
  useSearchShortcut(setSearching)
  if (snapshot.isError && snapshot.error instanceof APIError && snapshot.error.status === 401)
    return <SignIn />
  const failedConnections = failedConnectionCount(snapshot.data)
  return (
    <div className="app-shell">
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
        <main className="main-content" id="main-content">
          <WorkspaceContent snapshot={snapshot} />
        </main>
      </div>
      <SearchDialog
        deployments={snapshot.data?.deployments ?? []}
        open={searching}
        onOpenChange={setSearching}
      />
    </div>
  )
}
