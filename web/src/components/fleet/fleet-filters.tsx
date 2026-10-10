import { Search } from 'lucide-react'
import type { Snapshot } from '../../api/schema'
import type { FleetFilter } from '../../lib/filters'
import { collectionGaps } from '../../lib/filters'
import { collectionLabels, environmentLabels, healthLabels } from '../../lib/format'
import { Button } from '../ui/button'

interface SelectField {
  key: Exclude<keyof FleetFilter, 'query'>
  label: string
  options: readonly (readonly [string, string])[]
}

function selectFields(snapshot: Snapshot): readonly SelectField[] {
  return [
    {
      key: 'team',
      label: 'Team',
      options: [...new Set(snapshot.deployments.map((item) => item.team))]
        .sort()
        .map((team) => [team, team] as const),
    },
    { key: 'environment', label: 'Environment', options: Object.entries(environmentLabels) },
    {
      key: 'cluster',
      label: 'Cluster',
      options: snapshot.connections.map((connection) => [connection.id, connection.name] as const),
    },
    {
      key: 'health',
      label: 'State',
      options: (['critical', 'attention', 'unknown', 'healthy'] as const).map(
        (health) => [health, healthLabels[health]] as const,
      ),
    },
    {
      key: 'collection',
      label: 'Collection',
      options: [
        [collectionGaps, 'Any gap'],
        ...(['current', 'stale', 'unreachable', 'denied', 'unsupported'] as const).map(
          (state) => [state, collectionLabels[state]] as const,
        ),
      ],
    },
  ]
}

export function FleetFilters({
  snapshot,
  filter,
  onChange,
  onClear,
}: {
  snapshot: Snapshot
  filter: FleetFilter
  onChange: (field: keyof FleetFilter, value: string) => void
  onClear: () => void
}) {
  return (
    <div className="fleet-filters">
      <label className="search-field">
        <Search size={16} />
        <input
          aria-label="Search deployments"
          placeholder="Search deployments…"
          value={filter.query}
          onChange={(event) => {
            onChange('query', event.target.value)
          }}
        />
      </label>
      {selectFields(snapshot).map((field) => (
        <label key={field.key}>
          {field.label}
          <select
            aria-label={`Filter by ${field.label.toLowerCase()}`}
            value={filter[field.key]}
            onChange={(event) => {
              onChange(field.key, event.target.value)
            }}
          >
            <option value="">All</option>
            {field.options.map(([value, label]) => (
              <option value={value} key={value}>
                {label}
              </option>
            ))}
          </select>
        </label>
      ))}
      <Button
        size="small"
        variant="ghost"
        onClick={onClear}
        disabled={!Object.values(filter).some(Boolean)}
      >
        Clear filters
      </Button>
    </div>
  )
}
