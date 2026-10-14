import { Plus } from 'lucide-react'
import type { Configuration } from '../../api/schema'
import type { AccessGrant, GrantKind } from '../../lib/settings'
import { scopedNamespaceCount } from '../../lib/settings'
import { Button } from '../ui/button'

interface Row {
  kind: GrantKind
  grant: AccessGrant
  title: string
  detail: string
}

function rows(configuration: Configuration): Row[] {
  return [
    ...configuration.users.map((user) => ({
      kind: 'user' as const,
      grant: { key: user.subject, name: user.name, admin: user.admin, scopes: user.scopes },
      title: user.name || user.subject,
      detail: user.subject,
    })),
    ...configuration.groups.map((group) => ({
      kind: 'group' as const,
      grant: { key: group.name, name: '', admin: group.admin, scopes: group.scopes },
      title: group.name,
      detail: 'OIDC group',
    })),
  ]
}

function reach(grant: AccessGrant): string {
  return grant.admin ? 'Administrator' : `${String(scopedNamespaceCount(grant.scopes))} namespaces`
}

export function AccessSection({
  configuration,
  issuer,
  onAdd,
  onEdit,
  onRemove,
}: {
  configuration: Configuration
  issuer: string
  onAdd: (kind: GrantKind) => void
  onEdit: (kind: GrantKind, grant: AccessGrant) => void
  onRemove: (kind: GrantKind, grant: AccessGrant, label: string) => void
}) {
  return (
    <section>
      <div className="section-heading">
        <h2>Users and groups</h2>
        <div className="heading-actions">
          {(['user', 'group'] as const).map((kind) => (
            <Button
              key={kind}
              onClick={() => {
                onAdd(kind)
              }}
            >
              <Plus size={15} />
              Add {kind}
            </Button>
          ))}
        </div>
      </div>
      <p className="muted access-issuer">
        Users and groups match sign-ins from {issuer} only. Changing the issuer asks whether they
        carry over.
      </p>
      <div className="settings-list">
        {rows(configuration).map((row) => (
          <article key={`${row.kind}:${row.grant.key}`} className="settings-row">
            <div>
              <h3>{row.title}</h3>
              <p>{row.detail}</p>
            </div>
            <span>{reach(row.grant)}</span>
            <div className="row-actions">
              <Button
                size="small"
                onClick={() => {
                  onEdit(row.kind, row.grant)
                }}
              >
                Edit
              </Button>
              <Button
                size="small"
                variant="ghost"
                onClick={() => {
                  onRemove(row.kind, row.grant, row.title)
                }}
              >
                Remove
              </Button>
            </div>
          </article>
        ))}
      </div>
    </section>
  )
}
