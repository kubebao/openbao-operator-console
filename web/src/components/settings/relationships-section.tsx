import { GitBranch, Plus } from 'lucide-react'
import type { Configuration, ManagedRelationship } from '../../api/schema'
import { relationshipKindLabels } from '../../lib/format'
import { EmptyState } from '../page'
import { Button } from '../ui/button'

export function RelationshipsSection({
  configuration,
  onAdd,
  onEdit,
  onRemove,
}: {
  configuration: Configuration
  onAdd: () => void
  onEdit: (relationship: ManagedRelationship) => void
  onRemove: (relationship: ManagedRelationship) => void
}) {
  return (
    <section>
      <div className="section-heading">
        <h2>Declared relationships</h2>
        <Button onClick={onAdd}>
          <Plus size={15} />
          Add relationship
        </Button>
      </div>
      <div className="settings-list">
        {configuration.relationships.map((item) => (
          <article key={item.id} className="settings-row">
            <div>
              <h3>
                {item.source.namespace}/{item.source.name} → {item.target.namespace}/
                {item.target.name}
              </h3>
              <p>
                {relationshipKindLabels[item.kind]} · {item.id}
              </p>
            </div>
            <div className="row-actions">
              <Button
                size="small"
                onClick={() => {
                  onEdit(item)
                }}
              >
                Edit
              </Button>
              <Button
                size="small"
                variant="ghost"
                onClick={() => {
                  onRemove(item)
                }}
              >
                Remove
              </Button>
            </div>
          </article>
        ))}
      </div>
      {configuration.relationships.length === 0 && (
        <EmptyState title="No relationships configured" icon={GitBranch}>
          Select observed deployments to add a replication or recovery path.
        </EmptyState>
      )}
    </section>
  )
}
