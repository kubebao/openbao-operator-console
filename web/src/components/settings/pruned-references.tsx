import type { PrunedReferences } from '../../lib/settings'
import { hasPrunedReferences } from '../../lib/settings'

// PrunedReferenceList names the access grants and relationships a save removes because
// they reference namespaces that are no longer enrolled.
export function PrunedReferenceList({ references }: { references: PrunedReferences }) {
  if (!hasPrunedReferences(references)) return null
  return (
    <div className="pruned-references">
      {references.grants.length > 0 && (
        <>
          <h3>Access removed</h3>
          <ul aria-label="Access removed">
            {references.grants.map((grant) => (
              <li key={`${grant.kind}:${grant.key}`}>
                <strong>{grant.label}</strong> ({grant.kind === 'user' ? 'user' : 'group'}) loses{' '}
                {grant.namespaces.join(', ')}
              </li>
            ))}
          </ul>
        </>
      )}
      {references.relationships.length > 0 && (
        <>
          <h3>Relationships removed</h3>
          <ul aria-label="Relationships removed">
            {references.relationships.map((relationship) => (
              <li key={relationship.id}>
                <strong>{relationship.id}</strong>: {relationship.source.namespace}/
                {relationship.source.name} → {relationship.target.namespace}/
                {relationship.target.name}
              </li>
            ))}
          </ul>
        </>
      )}
    </div>
  )
}
