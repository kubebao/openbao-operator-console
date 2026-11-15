import type { useCharts } from '../../api/client'
import { Button } from '../ui/button'

export function ChartStatus({ query }: { query: ReturnType<typeof useCharts> }) {
  if (query.isError)
    return (
      <p role="status">
        {query.data
          ? 'Charts could not refresh. Previously loaded evidence remains visible.'
          : 'Chart evidence is unavailable.'}{' '}
        <Button onClick={() => void query.refetch()}>Retry charts</Button>
      </p>
    )
  if (query.isPending) return <p className="muted">Loading chart evidence…</p>
  return null
}
