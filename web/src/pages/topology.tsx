import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ArrowRight, GitBranch } from 'lucide-react'
import { useSnapshot } from '../api/client'
import type { Relationship, Snapshot } from '../api/schema'
import { EmptyState, PageHeading } from '../components/page'
import { RelationshipDetail } from '../components/topology/relationship-detail'
import { provenanceLabels, relationshipKindLabels } from '../lib/format'

type KindFilter = 'all' | Relationship['kind']

const kindFilters: readonly { kind: KindFilter; label: string }[] = [
  { kind: 'all', label: 'All relationships' },
  { kind: 'replication', label: 'Read replicas' },
  { kind: 'snapshotRecovery', label: 'Recovery' },
]

function ofKind(relationships: readonly Relationship[], kind: KindFilter): Relationship[] {
  return relationships.filter((relationship) => kind === 'all' || relationship.kind === kind)
}

function RelationshipList({
  relationships,
  selected,
  snapshot,
  onSelect,
}: {
  relationships: readonly Relationship[]
  selected: Relationship
  snapshot: Snapshot
  onSelect: (id: string) => void
}) {
  const find = (id: string) => snapshot.deployments.find((deployment) => deployment.id === id)
  return (
    <nav className="relationship-list" aria-label="Relationships">
      {relationships.map((item) => {
        const from = find(item.sourceId)
        const to = find(item.targetId)
        return (
          <button
            key={item.id}
            type="button"
            aria-current={item.id === selected.id ? 'true' : undefined}
            className={item.id === selected.id ? 'selected' : ''}
            onClick={() => {
              onSelect(item.id)
            }}
          >
            <span className="sr-only">Select </span>
            <strong>
              {from?.namespace}/{from?.name}
              <ArrowRight size={14} />
              <span className="sr-only"> to </span>
              {to?.namespace}/{to?.name}
            </strong>
            <small>
              {relationshipKindLabels[item.kind]} · {provenanceLabels[item.provenance]}
            </small>
          </button>
        )
      })}
    </nav>
  )
}

export function TopologyPage() {
  const { data: snapshot } = useSnapshot()
  const search = useSearch({ from: '/topology' })
  const navigate = useNavigate()
  if (!snapshot) return null
  const relationships = ofKind(snapshot.relationships, search.kind)
  const relationship =
    relationships.find((item) => item.id === search.relationship) ?? relationships[0]
  const find = (id: string | undefined) =>
    snapshot.deployments.find((deployment) => deployment.id === id)
  const source = find(relationship?.sourceId)
  const target = find(relationship?.targetId)
  const selected =
    [source, target].find((deployment) => deployment?.id === search.deployment) ?? source
  return (
    <div className="page-enter">
      <PageHeading title="Topology" />
      <div className="table-toolbar">
        <div className="segmented" role="group" aria-label="Relationship type">
          {kindFilters.map((filter) => (
            <button
              key={filter.kind}
              type="button"
              aria-pressed={search.kind === filter.kind}
              className={search.kind === filter.kind ? 'selected' : ''}
              onClick={() => {
                void navigate({
                  to: '/topology',
                  search: { kind: filter.kind, relationship: '', deployment: '' },
                })
              }}
            >
              {filter.label}
              <span>{ofKind(snapshot.relationships, filter.kind).length}</span>
            </button>
          ))}
        </div>
        {snapshot.viewer.admin && (
          <Link to="/settings" search={{ section: 'relationships' }} className="text-link">
            Manage relationships
            <ArrowRight size={14} />
          </Link>
        )}
      </div>
      {relationship && source && target && selected ? (
        <div className="relationship-workspace">
          <RelationshipList
            relationships={relationships}
            selected={relationship}
            snapshot={snapshot}
            onSelect={(id) => {
              void navigate({
                to: '/topology',
                search: { ...search, relationship: id, deployment: '' },
              })
            }}
          />
          <RelationshipDetail
            relationship={relationship}
            snapshot={snapshot}
            source={source}
            target={target}
            selected={selected}
            onSelect={(id) => {
              void navigate({
                to: '/topology',
                search: { ...search, relationship: relationship.id, deployment: id },
              })
            }}
          />
        </div>
      ) : (
        <EmptyState title="No relationships configured" icon={GitBranch}>
          {snapshot.viewer.admin ? (
            <Link to="/settings" search={{ section: 'relationships' }}>
              Add a relationship
            </Link>
          ) : (
            'No relationships are configured within your access scope.'
          )}
        </EmptyState>
      )}
    </div>
  )
}
