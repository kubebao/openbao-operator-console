import { AlertTriangle } from 'lucide-react'
import { Button } from '../ui/button'

// RefreshBanner reports a failed background refresh while the last loaded data stays usable.
// Open editors and typed input remain mounted.
export function RefreshBanner({ retrying, onRetry }: { retrying: boolean; onRetry: () => void }) {
  return (
    <div className="notice notice-warning refresh-banner" role="status">
      <AlertTriangle size={17} aria-hidden="true" />
      <span>
        <strong>The console could not refresh.</strong> The values shown were loaded earlier and may
        be outdated.
      </span>
      <Button size="small" disabled={retrying} onClick={onRetry}>
        {retrying ? 'Retrying…' : 'Retry now'}
      </Button>
    </div>
  )
}
