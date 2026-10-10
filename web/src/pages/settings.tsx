import { useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { AlertCircle, Lock } from 'lucide-react'
import { useCredentialStatuses, useSaveSettings, useSettings, useSnapshot } from '../api/client'
import type {
  Configuration,
  CredentialUpdate,
  ManagedConnection,
  ManagedRelationship,
  Settings,
  Snapshot,
} from '../api/schema'
import { AccessEditor } from '../components/access-editor'
import { ConnectionEditor } from '../components/connection-editor'
import { FormError } from '../components/form'
import { EmptyState, PageHeading } from '../components/page'
import { RelationshipEditor } from '../components/relationship-editor'
import { AccessSection } from '../components/settings/access-section'
import { ChangesSection } from '../components/settings/changes-section'
import { ConnectionsSection } from '../components/settings/connections-section'
import { IdentitySettings } from '../components/settings/identity-settings'
import { PolicySettings } from '../components/settings/policy-settings'
import { RelationshipsSection } from '../components/settings/relationships-section'
import { SettingsEditor } from '../components/settings-editor'
import { Button } from '../components/ui/button'
import { formError } from '../lib/errors'
import type { AccessGrant, EditMode, GrantKind } from '../lib/settings'
import {
  pruneScopes,
  toggleConnection,
  withGrant,
  withoutConnection,
  withoutGrant,
  withoutRelationship,
  withRelationship,
} from '../lib/settings'

const sections = [
  'connections',
  'access',
  'relationships',
  'policies',
  'identity',
  'changes',
] as const
type Section = (typeof sections)[number]
const sectionLabels: Record<Section, string> = {
  connections: 'Clusters',
  access: 'Access',
  relationships: 'Relationships',
  policies: 'Policies',
  identity: 'Identity provider',
  changes: 'Changes',
}

// Editing is the open settings dialog, if any.
type Editing =
  | { kind: 'connection'; connection: ManagedConnection }
  | { kind: 'access'; grantKind: GrantKind; mode: EditMode; grant: AccessGrant }
  | { kind: 'relationship'; mode: EditMode; relationship: ManagedRelationship | null }
  | { kind: 'remove'; label: string; configuration: Configuration }

export function SettingsPage() {
  const { data: snapshot } = useSnapshot()
  if (!snapshot) return null
  if (!snapshot.viewer.admin)
    return (
      <EmptyState title="Administrator access required" icon={Lock}>
        Console administrators manage clusters, access, and policies.{' '}
        <Link to="/clusters" className="text-link">
          View cluster coverage
        </Link>
      </EmptyState>
    )
  return <AdministrationPage snapshot={snapshot} />
}

function AdministrationPage({ snapshot }: { snapshot: Snapshot }) {
  const query = useSettings(true)
  const search = useSearch({ from: '/settings' })
  const [editing, setEditing] = useState<Editing | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState('')
  if (query.isPending) return <p className="loading-state">Loading settings…</p>
  if (query.isError)
    return (
      <EmptyState title="Settings unavailable" icon={AlertCircle}>
        {formError(query.error)}{' '}
        <Button
          onClick={() => {
            void query.refetch()
          }}
        >
          Reload
        </Button>
      </EmptyState>
    )
  return (
    <SettingsWorkspace
      settings={query.data}
      snapshot={snapshot}
      section={search.section}
      editing={editing}
      error={error}
      notice={notice}
      onEditing={setEditing}
      onError={setError}
      onNotice={setNotice}
      onReload={() => {
        setEditing(null)
        setError(null)
        setNotice('')
        void query.refetch()
      }}
    />
  )
}

function SettingsWorkspace({
  settings,
  snapshot,
  section,
  editing,
  error,
  notice,
  onEditing,
  onError,
  onNotice,
  onReload,
}: {
  settings: Settings
  snapshot: Snapshot
  section: Section
  editing: Editing | null
  error: unknown
  notice: string
  onEditing: (editing: Editing | null) => void
  onError: (error: unknown) => void
  onNotice: (notice: string) => void
  onReload: () => void
}) {
  const credentials = useCredentialStatuses(true)
  const save = useSaveSettings()
  const configuration = settings.configuration
  async function commit(next: Configuration, updates: readonly CredentialUpdate[] = []) {
    onNotice('')
    onError(null)
    await save.mutateAsync({
      revision: settings.revision,
      configuration: next,
      credentials: updates,
    })
    onNotice('Saved.')
  }
  function remove(label: string, next: Configuration) {
    onEditing({ kind: 'remove', label, configuration: pruneScopes(next) })
  }
  return (
    <div className="page-enter">
      <PageHeading
        title="Settings"
        actions={
          <Button disabled={save.isPending} onClick={onReload}>
            Reload settings
          </Button>
        }
      />
      <SectionTabs section={section} />
      {notice && (
        <p role="status" className="save-notice">
          {notice}
        </p>
      )}
      <FormError error={error}>
        <Button size="small" onClick={onReload}>
          Reload settings
        </Button>
      </FormError>
      {section === 'connections' && (
        <ConnectionsSection
          configuration={configuration}
          snapshot={snapshot}
          credentials={credentials.data?.credentials ?? []}
          credentialsUnavailable={credentials.isError}
          pending={save.isPending}
          onToggle={(item) => {
            void commit(toggleConnection(configuration, item.id)).catch(onError)
          }}
          onEdit={(connection) => {
            onEditing({ kind: 'connection', connection })
          }}
          onRemove={(item) => {
            remove(item.name, withoutConnection(configuration, item.id))
          }}
        />
      )}
      {section === 'access' && (
        <AccessSection
          configuration={configuration}
          onAdd={(grantKind) => {
            const grant = { key: '', name: '', admin: false, scopes: [] }
            onEditing({ kind: 'access', grantKind, mode: 'add', grant })
          }}
          onEdit={(grantKind, grant) => {
            onEditing({ kind: 'access', grantKind, mode: 'edit', grant })
          }}
          onRemove={(grantKind, grant, label) => {
            remove(label, withoutGrant(configuration, grantKind, grant.key))
          }}
        />
      )}
      {section === 'relationships' && (
        <RelationshipsSection
          configuration={configuration}
          onAdd={() => {
            onEditing({ kind: 'relationship', mode: 'add', relationship: null })
          }}
          onEdit={(relationship) => {
            onEditing({ kind: 'relationship', mode: 'edit', relationship })
          }}
          onRemove={(item) => {
            remove(item.id, withoutRelationship(configuration, item.id))
          }}
        />
      )}
      {section === 'policies' && (
        <PolicySettings key={settings.revision} settings={settings} onSave={commit} />
      )}
      {section === 'identity' && <IdentitySettings key={settings.revision} settings={settings} />}
      {section === 'changes' && <ChangesSection changes={settings.changes} />}
      {editing && (
        <SettingsDialog
          editing={editing}
          configuration={configuration}
          snapshot={snapshot}
          pending={save.isPending}
          commit={commit}
          onError={onError}
          onClose={() => {
            onEditing(null)
          }}
        />
      )}
    </div>
  )
}

function SectionTabs({ section }: { section: Section }) {
  const navigate = useNavigate()
  return (
    <nav className="settings-tabs" aria-label="Settings sections">
      {sections.map((value) => (
        <button
          key={value}
          className={section === value ? 'selected' : ''}
          onClick={() => {
            void navigate({ to: '/settings', search: { section: value } })
          }}
        >
          {sectionLabels[value]}
        </button>
      ))}
    </nav>
  )
}

function SettingsDialog({
  editing,
  configuration,
  snapshot,
  pending,
  commit,
  onError,
  onClose,
}: {
  editing: Editing
  configuration: Configuration
  snapshot: Snapshot
  pending: boolean
  commit: (configuration: Configuration, credentials?: readonly CredentialUpdate[]) => Promise<void>
  onError: (error: unknown) => void
  onClose: () => void
}) {
  switch (editing.kind) {
    case 'connection':
      return (
        <ConnectionEditor
          initial={editing.connection}
          onClose={onClose}
          onSave={async (next, credential) => {
            const connections = configuration.connections.map((item) =>
              item.id === next.id ? next : item,
            )
            await commit(
              pruneScopes({ ...configuration, connections }),
              credential ? [credential] : [],
            )
          }}
        />
      )
    case 'access':
      return (
        <AccessEditor
          kind={editing.grantKind}
          mode={editing.mode}
          initial={editing.grant}
          configuration={configuration}
          onClose={onClose}
          onSave={(grant) =>
            commit(withGrant(configuration, editing.grantKind, editing.mode, grant))
          }
        />
      )
    case 'relationship':
      return (
        <RelationshipEditor
          initial={editing.relationship}
          mode={editing.mode}
          deployments={snapshot.deployments}
          onClose={onClose}
          onSave={(relationship) =>
            commit(withRelationship(configuration, editing.mode, relationship))
          }
        />
      )
    case 'remove':
      return (
        <SettingsEditor title={`Remove ${editing.label}?`} onClose={onClose}>
          <p>
            This removes the configuration and any matching access grants or relationships.
            Collected history is retained.
          </p>
          <div className="editor-actions">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={pending}
              onClick={() => {
                void commit(editing.configuration).catch(onError).finally(onClose)
              }}
            >
              Remove
            </Button>
          </div>
        </SettingsEditor>
      )
  }
}
