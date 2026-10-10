import { Outlet } from '@tanstack/react-router'
import type { UseQueryResult } from '@tanstack/react-query'
import { Layers3 } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import { Button } from '../ui/button'

// WorkspaceContent renders the active page once fleet evidence is available. A 401 is
// handled before this point by the sign-in screen.
export function WorkspaceContent({ snapshot }: { snapshot: UseQueryResult<Snapshot> }) {
  if (snapshot.isPending)
    return (
      <div className="loading-state">
        <span className="loading-dot" />
        Loading fleet evidence…
      </div>
    )
  if (snapshot.isError)
    return (
      <div className="error-state">
        <Layers3 size={30} />
        <h1>Fleet evidence unavailable</h1>
        <p>The console cannot refresh evidence. Existing workloads continue independently.</p>
        <Button
          onClick={() => {
            void snapshot.refetch()
          }}
        >
          Try again
        </Button>
      </div>
    )
  return (
    <>
      {snapshot.data.mode === 'demo' && (
        <div className="workspace-notice">
          <span>DEMONSTRATION</span>Illustrative evidence · no live clusters connected
        </div>
      )}
      <Outlet />
    </>
  )
}
