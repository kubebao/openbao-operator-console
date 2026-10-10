import { formError } from '../lib/errors'
import { useState } from 'react'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { AlertCircle, Cable, GitBranch, Lock, Plus, Pause, Play } from 'lucide-react'
import {
  useSettings,
  useSnapshot,
  saveSettings,
  trialIdentity,
  useCredentialStatuses,
} from '../api/client'
import type {
  Configuration,
  CredentialUpdate,
  Settings,
  ManagedConnection,
  ManagedRelationship,
  IdentityInput,
} from '../api/schema'
import { PageHeading, EmptyState } from '../components/page'
import { Button } from '../components/ui/button'
import { ConnectionEditor } from '../components/connection-editor'
import { AccessEditor } from '../components/access-editor'
import type { AccessGrant } from '../components/access-editor'
import { RelationshipEditor } from '../components/relationship-editor'
import { SettingsEditor } from '../components/settings-editor'
import { Field } from '../components/form'
import { CredentialStatusLabel } from '../components/credential-renewal'
import { collectionLabels } from '../lib/format'
import type { CollectionState } from '../lib/format'
import { Timestamp } from '../components/status'

const sections = [
  'connections',
  'access',
  'relationships',
  'policies',
  'identity',
  'changes',
] as const
const labels = {
  connections: 'Clusters',
  access: 'Access',
  relationships: 'Relationships',
  policies: 'Policies',
  identity: 'Identity provider',
  changes: 'Changes',
}
function pruneScopes(configuration: Configuration): Configuration {
  const scopes = (items: Configuration['users'][number]['scopes']) =>
    items
      .map((scope) => ({
        ...scope,
        namespaces: scope.namespaces.filter((namespace) =>
          configuration.connections.some(
            (connection) =>
              connection.id === scope.connectionId &&
              connection.namespaces.some((item) => item.name === namespace),
          ),
        ),
      }))
      .filter((scope) => scope.namespaces.length)
  return {
    ...configuration,
    users: configuration.users.map((user) => ({ ...user, scopes: scopes(user.scopes) })),
    groups: configuration.groups.map((group) => ({ ...group, scopes: scopes(group.scopes) })),
    relationships: configuration.relationships.filter((relationship) =>
      [relationship.source, relationship.target].every((resource) =>
        configuration.connections.some(
          (connection) =>
            connection.id === resource.connectionId &&
            connection.namespaces.some((namespace) => namespace.name === resource.namespace),
        ),
      ),
    ),
  }
}
function collectionTone(state: CollectionState | 'paused'): string {
  if (state === 'current') return 'status-healthy'
  if (state === 'pending' || state === 'paused') return 'status-unknown'
  return 'status-attention'
}

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
  return <AdministrationPage />
}

function AdministrationPage() {
  const query = useSettings(true)
  const { data: snapshot } = useSnapshot()
  const credentials = useCredentialStatuses(snapshot?.viewer.admin === true)
  const search = useSearch({ from: '/settings' })
  const navigate = useNavigate()
  const client = useQueryClient()
  const [connection, setConnection] = useState<ManagedConnection | null>(null)
  const [access, setAccess] = useState<{
    kind: 'user' | 'group'
    existing: boolean
    grant: AccessGrant
  } | null>(null)
  const [relationship, setRelationship] = useState<ManagedRelationship | null>(null)
  const [newRelationship, setNewRelationship] = useState(false)
  const [removal, setRemoval] = useState<{ label: string; configuration: Configuration } | null>(
    null,
  )
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  const [notice, setNotice] = useState('')
  async function commit(
    configuration: Configuration,
    credentials: readonly CredentialUpdate[] = [],
  ) {
    if (!query.data) throw new Error('Reload settings before saving.')
    setPending(true)
    setNotice('')
    setError('')
    try {
      const result = await saveSettings(query.data.revision, configuration, credentials)
      client.setQueryData(['settings'], result)
      await client.invalidateQueries({ queryKey: ['snapshot'] })
      await client.invalidateQueries({ queryKey: ['credentialStatuses'] })
      setNotice('Saved.')
      return result
    } finally {
      setPending(false)
    }
  }
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
  const settings = query.data
  const configuration = settings.configuration
  const section = search.section
  function remove(label: string, next: Configuration) {
    setRemoval({ label, configuration: pruneScopes(next) })
  }
  return (
    <div className="page-enter">
      <PageHeading
        title="Settings"
        actions={
          <Button
            disabled={pending}
            onClick={() => {
              setConnection(null)
              setAccess(null)
              setRelationship(null)
              setNewRelationship(false)
              setRemoval(null)
              setError('')
              setNotice('')
              void query.refetch()
            }}
          >
            Reload settings
          </Button>
        }
      />
      <nav className="settings-tabs" aria-label="Settings sections">
        {sections.map((value) => (
          <button
            key={value}
            className={section === value ? 'selected' : ''}
            onClick={() => {
              void navigate({ to: '/settings', search: { section: value } })
            }}
          >
            {labels[value]}
          </button>
        ))}
      </nav>
      {notice && (
        <p role="status" className="save-notice">
          {notice}
        </p>
      )}
      {error && (
        <div className="form-error" role="alert">
          {error}
          <Button
            size="small"
            onClick={() => {
              void query.refetch()
            }}
          >
            Reload settings
          </Button>
        </div>
      )}
      {section === 'connections' && (
        <section>
          <div className="section-heading">
            <h2>
              {configuration.connections.length}{' '}
              {configuration.connections.length === 1 ? 'cluster' : 'clusters'}
            </h2>
            <Button
              onClick={() => {
                void navigate({ to: '/settings/clusters/new' })
              }}
            >
              <Plus size={15} />
              Add cluster
            </Button>
          </div>
          <div className="settings-list">
            {configuration.connections.map((item) => {
              const observed = snapshot?.connections.find((observed) => observed.id === item.id)
              const credential = credentials.data?.credentials.find(
                (status) => status.connectionId === item.id,
              )
              return (
                <article key={item.id} className="settings-row">
                  <div>
                    <h3>{item.name}</h3>
                    <p>
                      {[
                        item.authentication === 'local' ? 'Hosting cluster' : item.server,
                        item.region,
                        item.id,
                      ]
                        .filter(Boolean)
                        .join(' · ')}
                    </p>
                    <div className="namespace-tags">
                      {item.namespaces.map((namespace) => (
                        <span key={namespace.name}>{namespace.name}</span>
                      ))}
                    </div>
                  </div>
                  <div className="settings-row-state">
                    <span
                      className={`badge ${collectionTone(item.disabled ? 'paused' : (observed?.collection ?? 'pending'))}`}
                    >
                      <span className="status-dot" />
                      {item.disabled
                        ? 'Paused'
                        : observed?.collection === 'current'
                          ? 'Connected'
                          : collectionLabels[observed?.collection ?? 'pending']}
                    </span>
                    <small>
                      {item.disabled ? (
                        'Collection stopped'
                      ) : observed?.lastSuccessAt ? (
                        <>
                          Last collected <Timestamp value={observed.lastSuccessAt} />
                        </>
                      ) : (
                        'No successful collection yet'
                      )}
                    </small>
                    <CredentialStatusLabel status={credential} />
                    {item.renewal && credentials.isError && (
                      <small>Renewal status unavailable</small>
                    )}
                  </div>
                  <div className="row-actions">
                    <Button
                      size="small"
                      variant="ghost"
                      disabled={pending}
                      aria-label={`${item.disabled ? 'Resume' : 'Pause'} ${item.name}`}
                      onClick={() => {
                        void commit({
                          ...configuration,
                          connections: configuration.connections.map((connection) =>
                            connection.id === item.id
                              ? { ...connection, disabled: !connection.disabled }
                              : connection,
                          ),
                        }).catch((error: unknown) => {
                          setError(formError(error))
                        })
                      }}
                    >
                      {item.disabled ? <Play size={14} /> : <Pause size={14} />}
                      {item.disabled ? 'Resume' : 'Pause'}
                    </Button>
                    <Button
                      size="small"
                      onClick={() => {
                        setConnection(item)
                      }}
                    >
                      {credential?.state === 'reconnectRequired' ? 'Reconnect' : 'Edit'}
                    </Button>
                    <Button
                      size="small"
                      variant="ghost"
                      onClick={() => {
                        remove(item.name, {
                          ...configuration,
                          connections: configuration.connections.filter(
                            (connection) => connection.id !== item.id,
                          ),
                        })
                      }}
                    >
                      Remove
                    </Button>
                  </div>
                </article>
              )
            })}
          </div>
          {configuration.connections.length === 0 && (
            <EmptyState title="No clusters connected" icon={Cable}>
              Add a cluster to start collecting OpenBao resources.
            </EmptyState>
          )}
        </section>
      )}
      {section === 'access' && (
        <section>
          <div className="section-heading">
            <h2>Users and groups</h2>
            <div className="heading-actions">
              {(['user', 'group'] as const).map((kind) => (
                <Button
                  key={kind}
                  onClick={() => {
                    setAccess({
                      kind,
                      existing: false,
                      grant: { key: '', name: '', admin: false, scopes: [] },
                    })
                  }}
                >
                  <Plus size={15} />
                  Add {kind}
                </Button>
              ))}
            </div>
          </div>
          <div className="settings-list">
            {configuration.users.map((user) => (
              <article key={user.subject} className="settings-row">
                <div>
                  <h3>{user.name || user.subject}</h3>
                  <p>{user.subject}</p>
                </div>
                <span>
                  {user.admin
                    ? 'Administrator'
                    : `${String(user.scopes.reduce((count, scope) => count + scope.namespaces.length, 0))} namespaces`}
                </span>
                <div className="row-actions">
                  <Button
                    size="small"
                    onClick={() => {
                      setAccess({
                        kind: 'user',
                        existing: true,
                        grant: {
                          key: user.subject,
                          name: user.name,
                          admin: user.admin,
                          scopes: user.scopes,
                        },
                      })
                    }}
                  >
                    Edit
                  </Button>
                  <Button
                    size="small"
                    variant="ghost"
                    onClick={() => {
                      remove(user.name || user.subject, {
                        ...configuration,
                        users: configuration.users.filter((item) => item.subject !== user.subject),
                      })
                    }}
                  >
                    Remove
                  </Button>
                </div>
              </article>
            ))}
            {configuration.groups.map((group) => (
              <article key={group.name} className="settings-row">
                <div>
                  <h3>{group.name}</h3>
                  <p>OIDC group</p>
                </div>
                <span>
                  {group.admin
                    ? 'Administrator'
                    : `${String(group.scopes.reduce((count, scope) => count + scope.namespaces.length, 0))} namespaces`}
                </span>
                <div className="row-actions">
                  <Button
                    size="small"
                    onClick={() => {
                      setAccess({
                        kind: 'group',
                        existing: true,
                        grant: {
                          key: group.name,
                          name: '',
                          admin: group.admin,
                          scopes: group.scopes,
                        },
                      })
                    }}
                  >
                    Edit
                  </Button>
                  <Button
                    size="small"
                    variant="ghost"
                    onClick={() => {
                      remove(group.name, {
                        ...configuration,
                        groups: configuration.groups.filter((item) => item.name !== group.name),
                      })
                    }}
                  >
                    Remove
                  </Button>
                </div>
              </article>
            ))}
          </div>
        </section>
      )}
      {section === 'relationships' && (
        <section>
          <div className="section-heading">
            <h2>Declared relationships</h2>
            <Button
              onClick={() => {
                setNewRelationship(true)
                setRelationship(null)
              }}
            >
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
                    {item.kind === 'replication' ? 'Remote read replica' : 'Snapshot recovery'} ·{' '}
                    {item.id}
                  </p>
                </div>
                <div className="row-actions">
                  <Button
                    size="small"
                    onClick={() => {
                      setNewRelationship(false)
                      setRelationship(item)
                    }}
                  >
                    Edit
                  </Button>
                  <Button
                    size="small"
                    variant="ghost"
                    onClick={() => {
                      remove(item.id, {
                        ...configuration,
                        relationships: configuration.relationships.filter(
                          (relationship) => relationship.id !== item.id,
                        ),
                      })
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
      )}
      {section === 'policies' && (
        <PolicySettings key={settings.revision} settings={settings} onSave={commit} />
      )}
      {section === 'identity' && <IdentitySettings key={settings.revision} settings={settings} />}
      {section === 'changes' && (
        <section className="table-surface">
          <table>
            <thead>
              <tr>
                <th>When</th>
                <th>Administrator</th>
                <th>Change</th>
                <th>Revision</th>
              </tr>
            </thead>
            <tbody>
              {settings.changes.map((change) => (
                <tr key={change.revision}>
                  <td>
                    <Timestamp value={change.at} />
                  </td>
                  <td>{change.actor}</td>
                  <td>{change.summary}</td>
                  <td>{change.revision}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}
      {connection && (
        <ConnectionEditor
          initial={connection}
          onClose={() => {
            setConnection(null)
          }}
          onSave={async (next, credential) => {
            await commit(
              pruneScopes({
                ...configuration,
                connections: configuration.connections.map((item) =>
                  item.id === next.id ? next : item,
                ),
              }),
              credential ? [credential] : [],
            )
          }}
        />
      )}
      {access && (
        <AccessEditor
          {...access}
          initial={access.grant}
          configuration={configuration}
          onClose={() => {
            setAccess(null)
          }}
          onSave={async (grant) => {
            if (access.kind === 'user') {
              const user = {
                subject: grant.key,
                name: grant.name,
                admin: grant.admin,
                scopes: grant.scopes,
              }
              await commit({
                ...configuration,
                users: access.existing
                  ? configuration.users.map((item) => (item.subject === grant.key ? user : item))
                  : [...configuration.users, user],
              })
            } else {
              const group = { name: grant.key, admin: grant.admin, scopes: grant.scopes }
              await commit({
                ...configuration,
                groups: access.existing
                  ? configuration.groups.map((item) => (item.name === grant.key ? group : item))
                  : [...configuration.groups, group],
              })
            }
          }}
        />
      )}
      {(newRelationship || relationship) && (
        <RelationshipEditor
          initial={relationship}
          existing={!newRelationship}
          deployments={snapshot?.deployments ?? []}
          onClose={() => {
            setNewRelationship(false)
            setRelationship(null)
          }}
          onSave={async (next) => {
            await commit({
              ...configuration,
              relationships: newRelationship
                ? [...configuration.relationships, next]
                : configuration.relationships.map((item) => (item.id === next.id ? next : item)),
            })
          }}
        />
      )}
      {removal && (
        <SettingsEditor
          title={`Remove ${removal.label}?`}
          onClose={() => {
            setRemoval(null)
          }}
        >
          <p>
            This removes the configuration and any matching access grants or relationships.
            Collected history is retained.
          </p>
          <div className="editor-actions">
            <Button
              variant="ghost"
              onClick={() => {
                setRemoval(null)
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={pending}
              onClick={() => {
                void commit(removal.configuration)
                  .then(() => {
                    setRemoval(null)
                  })
                  .catch((error: unknown) => {
                    setError(formError(error))
                    setRemoval(null)
                  })
              }}
            >
              Remove
            </Button>
          </div>
        </SettingsEditor>
      )}
    </div>
  )
}
function PolicySettings({
  settings,
  onSave,
}: {
  settings: Settings
  onSave: (configuration: Configuration) => Promise<Settings>
}) {
  const [hours, setHours] = useState(settings.configuration.maxBackupAgeHours)
  const [grace, setGrace] = useState(settings.configuration.startupGraceMinutes)
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        setPending(true)
        setError('')
        void onSave({
          ...settings.configuration,
          maxBackupAgeHours: hours,
          startupGraceMinutes: grace,
        })
          .catch((error: unknown) => {
            setError(formError(error))
          })
          .finally(() => {
            setPending(false)
          })
      }}
    >
      <h2>Attention policies</h2>
      <Field
        label="Maximum backup age (hours)"
        help="A configured backup older than this threshold needs attention."
      >
        <input
          type="number"
          required
          min={1}
          value={hours}
          onChange={(event) => {
            setHours(event.target.valueAsNumber)
          }}
        />
      </Field>
      <Field
        label="Startup grace (minutes)"
        help="Delay availability findings while a new deployment starts."
      >
        <input
          type="number"
          required
          min={0}
          value={grace}
          onChange={(event) => {
            setGrace(event.target.valueAsNumber)
          }}
        />
      </Field>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <Button type="submit" variant="default" disabled={pending}>
        {pending ? 'Saving…' : 'Save policies'}
      </Button>
    </form>
  )
}
function IdentitySettings({ settings }: { settings: Settings }) {
  const [identity, setIdentity] = useState<IdentityInput>({
    issuer: settings.identity.issuer,
    clientId: settings.identity.clientId,
    ca: settings.identity.ca,
    groupsClaim: settings.identity.groupsClaim,
  })
  const [secret, setSecret] = useState('')
  const [error, setError] = useState('')
  const [pending, setPending] = useState(false)
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        setPending(true)
        setError('')
        void trialIdentity(
          settings.revision,
          secret ? { ...identity, clientSecret: secret } : identity,
        )
          .then((url) => {
            window.location.assign(url)
          })
          .catch((error: unknown) => {
            setError(formError(error))
            setPending(false)
          })
      }}
    >
      <h2>OpenID Connect</h2>
      <Field label="Issuer">
        <input
          type="url"
          required
          value={identity.issuer}
          onChange={(event) => {
            setIdentity({ ...identity, issuer: event.target.value })
          }}
        />
      </Field>
      <Field label="Client ID">
        <input
          required
          value={identity.clientId}
          onChange={(event) => {
            setIdentity({ ...identity, clientId: event.target.value })
          }}
        />
      </Field>
      <Field
        label="Client secret"
        help={
          settings.identity.secretConfigured
            ? 'Leave empty to keep the secret for this issuer and client.'
            : 'Supply the confidential client secret.'
        }
      >
        <input
          type="password"
          autoComplete="new-password"
          value={secret}
          onChange={(event) => {
            setSecret(event.target.value)
          }}
        />
      </Field>
      <Field label="Groups claim" help="Top-level token claim containing an array of group names.">
        <input
          value={identity.groupsClaim}
          onChange={(event) => {
            setIdentity({ ...identity, groupsClaim: event.target.value })
          }}
        />
      </Field>
      <Field label="CA certificate" help="Leave empty to use system trust.">
        <textarea
          rows={4}
          value={identity.ca}
          onChange={(event) => {
            setIdentity({ ...identity, ca: event.target.value })
          }}
        />
      </Field>
      <p className="muted">
        Sign in through the candidate provider as an enrolled administrator. Activation follows a
        successful login and ends existing console sessions.
      </p>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      <Button type="submit" variant="default" disabled={pending}>
        {pending ? 'Checking…' : 'Verify login and activate'}
      </Button>
    </form>
  )
}
