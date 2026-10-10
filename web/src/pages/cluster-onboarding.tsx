import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  AlertCircle,
  ArrowLeft,
  Check,
  CircleDashed,
  Loader2,
  Lock,
  Pause,
  Plus,
  Trash2,
} from 'lucide-react'
import {
  checkConnection,
  APIError,
  generateReaderAccess,
  saveSettings,
  snapshotOptions,
  useSettings,
  useSnapshot,
} from '../api/client'
import type {
  Deployment,
  ConnectionCheck,
  ConnectionCheckResult,
  CredentialUpdate,
  ManagedConnection,
  ReaderAccessResult,
} from '../api/schema'
import { DetailRows, EmptyState, PageHeading } from '../components/page'
import { Field } from '../components/form'
import { Button } from '../components/ui/button'
import { formError } from '../lib/errors'
import { OnboardingChecks } from '../components/onboarding-checks'
import { RenewalFields } from '../components/credential-renewal'
import { OnboardingReader } from '../components/onboarding-reader'
import { EvidenceBadge, HealthBadge } from '../components/status'
import { collectionLabels } from '../lib/format'
import { emptyFilter } from '../lib/filters'

const steps = ['Cluster', 'Scope', 'Reader access', 'Credentials', 'Verify'] as const
type Step = (typeof steps)[number]
type CredentialMethod = 'kubeconfig' | 'token' | 'certificate'
type PendingAction = 'prepare' | 'check' | 'save' | 'reload'
const namespacePattern = /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/

export function ClusterOnboardingPage() {
  const { data: snapshot } = useSnapshot()
  const query = useSettings(snapshot?.viewer.admin === true)
  const client = useQueryClient()
  const navigate = useNavigate()
  const [suffix] = useState(() => crypto.randomUUID().slice(0, 8))
  const [connection, setConnection] = useState<ManagedConnection>({
    id: '',
    name: '',
    region: '',
    authentication: 'remote',
    server: '',
    credentialConfigured: false,
    disabled: false,
    namespaces: [{ name: '', team: '', environment: 'other', location: '' }],
    operators: [],
  })
  const [manualID, setManualID] = useState(false)
  const [step, setStep] = useState<Step>('Cluster')
  const [method, setMethod] = useState<CredentialMethod>('kubeconfig')
  const [context, setContext] = useState('')
  const [commandContext, setCommandContext] = useState('')
  const [kubeconfig, setKubeconfig] = useState('')
  const [token, setToken] = useState('')
  const [ca, setCA] = useState('')
  const [certificate, setCertificate] = useState('')
  const [key, setKey] = useState('')
  const [access, setAccess] = useState<{ fingerprint: string; result: ReaderAccessResult } | null>(
    null,
  )
  const [check, setCheck] = useState<{ fingerprint: string; result: ConnectionCheckResult } | null>(
    null,
  )
  const [pendingAction, setPendingAction] = useState<PendingAction | null>(null)
  const pending = pendingAction !== null
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [conflict, setConflict] = useState(false)
  const [saved, setSaved] = useState<{
    id: string
    paused: boolean
    preview: readonly Deployment[]
  } | null>(null)
  const local = connection.authentication === 'local'
  const stepIndex = steps.indexOf(step)
  const scopeFingerprint = JSON.stringify({ ...connection, server: '', disabled: false })
  const currentAccess = access?.fingerprint === scopeFingerprint ? access.result : null
  const input: ConnectionCheck = local
    ? { connection }
    : method === 'kubeconfig'
      ? { connection, kubeconfig, ...(context ? { context } : {}) }
      : {
          connection,
          credential: {
            server: connection.server,
            ca,
            token: method === 'token' ? token : '',
            certificate: method === 'certificate' ? certificate : '',
            key: method === 'certificate' ? key : '',
          },
        }
  const checkFingerprint = JSON.stringify(input)
  const currentCheck = check?.fingerprint === checkFingerprint ? check.result : null

  async function advance() {
    setError('')
    setNotice('')
    if (step === 'Cluster') {
      if (!connection.name.trim() || !/^[a-zA-Z0-9_-]{1,100}$/.test(connection.id)) {
        setError('Enter a cluster name and a valid identifier.')
        return
      }
      if (query.data?.configuration.connections.some((item) => item.id === connection.id)) {
        setError('This identifier is already enrolled. Choose a different identifier.')
        return
      }
    }
    if (step === 'Scope') {
      const names = connection.namespaces.map((namespace) => namespace.name)
      if (
        !names.length ||
        names.some((name) => name.length > 63 || !namespacePattern.test(name)) ||
        new Set(names).size !== names.length
      ) {
        setError('Enter at least one valid, unique Kubernetes namespace.')
        return
      }
      setPendingAction('prepare')
      try {
        setAccess({ fingerprint: scopeFingerprint, result: await generateReaderAccess(connection) })
      } catch (error) {
        setError(formError(error))
        return
      } finally {
        setPendingAction(null)
      }
    }
    if (
      step === 'Credentials' &&
      !local &&
      ((method === 'kubeconfig' && !kubeconfig.trim()) ||
        (method !== 'kubeconfig' &&
          (!connection.server || (method === 'token' ? !token : !certificate || !key))))
    ) {
      setError('Supply a complete reader credential before verification.')
      return
    }
    const next = steps[stepIndex + 1]
    if (next) setStep(next)
  }

  async function test() {
    setPendingAction('check')
    setError('')
    setCheck(null)
    try {
      setCheck({ fingerprint: checkFingerprint, result: await checkConnection(input) })
    } catch (error) {
      setError(formError(error))
    } finally {
      setPendingAction(null)
    }
  }

  async function connect(paused: boolean) {
    if (!query.data) return
    if (query.data.configuration.connections.some((item) => item.id === connection.id)) {
      setError(
        'This identifier is already enrolled. Return to Cluster and choose a different identifier.',
      )
      return
    }
    if (!paused && currentCheck?.connection.collection !== 'current') {
      setError('Verify reader access before connecting, or save this cluster paused.')
      return
    }
    setPendingAction('save')
    setError('')
    try {
      const credentials: CredentialUpdate[] = []
      if (input.credential)
        credentials.push({ connectionId: connection.id, credential: input.credential })
      if (input.kubeconfig)
        credentials.push({
          connectionId: connection.id,
          kubeconfig: input.kubeconfig,
          ...(input.context ? { context: input.context } : {}),
        })
      const result = await saveSettings(
        query.data.revision,
        {
          ...query.data.configuration,
          connections: [
            ...query.data.configuration.connections,
            { ...connection, disabled: paused },
          ],
        },
        credentials,
      )
      client.setQueryData(['settings'], result)
      await client.invalidateQueries({ queryKey: ['snapshot'] })
      setToken('')
      setCertificate('')
      setKey('')
      setKubeconfig('')
      setCA('')
      setCheck(null)
      setAccess(null)
      setSaved({ id: connection.id, paused, preview: currentCheck?.deployments ?? [] })
    } catch (error) {
      setError(formError(error))
      setConflict(error instanceof APIError && error.status === 409)
    } finally {
      setPendingAction(null)
    }
  }

  if (snapshot && !snapshot.viewer.admin)
    return (
      <EmptyState title="Administrator access required" icon={Lock}>
        A console administrator can add clusters.
      </EmptyState>
    )
  if (query.isError)
    return (
      <EmptyState title="Settings unavailable" icon={AlertCircle}>
        {formError(query.error)}{' '}
        <Button
          onClick={() => {
            void query.refetch()
          }}
        >
          Reload settings
        </Button>
      </EmptyState>
    )
  if (!query.data) return <p className="loading-state">Loading settings…</p>
  if (saved)
    return (
      <ConnectedSummary
        id={saved.id}
        name={connection.name}
        paused={saved.paused}
        preview={saved.preview}
      />
    )
  return (
    <div className="page-enter onboarding-layout">
      <PageHeading
        title="Add cluster"
        actions={
          <Button
            variant="ghost"
            onClick={() => {
              void navigate({ to: '/settings', search: { section: 'connections' } })
            }}
          >
            Cancel
          </Button>
        }
      />
      <ol className="onboarding-steps" aria-label="Add cluster steps">
        {steps.map((name, index) => (
          <li
            key={name}
            aria-current={name === step ? 'step' : undefined}
            className={index < stepIndex ? 'complete' : name === step ? 'current' : ''}
          >
            <span>{index < stepIndex ? <Check size={14} /> : index + 1}</span>
            {name}
          </li>
        ))}
      </ol>
      <form
        className="onboarding-workspace"
        onSubmit={(event) => {
          event.preventDefault()
          void advance()
        }}
      >
        <fieldset disabled={pending} className="onboarding-content">
          <legend className="sr-only">{step}</legend>
          {step === 'Cluster' && (
            <>
              <h2>Which cluster do you want to observe?</h2>
              <Field label="Cluster name">
                <input
                  required
                  autoFocus
                  value={connection.name}
                  onChange={(event) => {
                    const name = event.target.value
                    const slug = name
                      .toLowerCase()
                      .replace(/[^a-z0-9]+/g, '-')
                      .replace(/^-|-$/g, '')
                      .slice(0, 70)
                    setConnection({
                      ...connection,
                      name,
                      ...(!manualID ? { id: `${slug || 'cluster'}-${suffix}` } : {}),
                    })
                  }}
                  placeholder="Production EU"
                />
              </Field>
              <Field label="Connection">
                <select
                  value={connection.authentication}
                  onChange={(event) => {
                    const authentication = event.target.value
                    if (authentication === 'local' || authentication === 'remote')
                      setConnection({
                        ...connection,
                        authentication,
                        ...(authentication === 'local' ? { renewal: null } : {}),
                      })
                  }}
                >
                  <option value="remote">Remote Kubernetes cluster</option>
                  <option value="local">Cluster hosting this console</option>
                </select>
              </Field>
              {local && (
                <p className="onboarding-help">
                  Uses the console's Kubernetes ServiceAccount. No credential upload is required.
                </p>
              )}
              <details>
                <summary>Advanced</summary>
                <Field
                  label="Cluster identifier"
                  help="Stable identity for access rules and history. It cannot be renamed later."
                >
                  <input
                    required
                    pattern="[a-zA-Z0-9_-]{1,100}"
                    value={connection.id}
                    onChange={(event) => {
                      setManualID(true)
                      setConnection({ ...connection, id: event.target.value })
                    }}
                  />
                </Field>
              </details>
            </>
          )}
          {step === 'Scope' && (
            <>
              <h2>Choose namespaces</h2>
              <p className="onboarding-help">The reader only observes the namespaces you enter.</p>
              {connection.namespaces.map((namespace, index) => (
                <div className="onboarding-namespace" key={index}>
                  <Field label={`Namespace ${String(index + 1)}`}>
                    <input
                      required
                      pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?"
                      maxLength={63}
                      value={namespace.name}
                      onChange={(event) => {
                        setConnection({
                          ...connection,
                          namespaces: connection.namespaces.map((item, position) =>
                            position === index ? { ...item, name: event.target.value } : item,
                          ),
                          operators: connection.operators.map((operator) => ({
                            ...operator,
                            namespaces: connection.namespaces
                              .map((item, position) =>
                                position === index ? event.target.value : item.name,
                              )
                              .filter(Boolean),
                          })),
                        })
                      }}
                      placeholder="openbao-production"
                    />
                  </Field>
                  <Button
                    type="button"
                    size="icon"
                    variant="ghost"
                    aria-label={`Remove namespace ${String(index + 1)}`}
                    disabled={connection.namespaces.length === 1}
                    onClick={() => {
                      setConnection({
                        ...connection,
                        namespaces: connection.namespaces.filter(
                          (_, position) => position !== index,
                        ),
                        operators: connection.operators.map((operator) => ({
                          ...operator,
                          namespaces: operator.namespaces.filter((name) => name !== namespace.name),
                        })),
                      })
                    }}
                  >
                    <Trash2 size={15} />
                  </Button>
                </div>
              ))}
              <Button
                type="button"
                size="small"
                onClick={() => {
                  setConnection({
                    ...connection,
                    namespaces: [
                      ...connection.namespaces,
                      { name: '', team: '', environment: 'other', location: '' },
                    ],
                    operators: connection.operators.map((operator) => ({
                      ...operator,
                      namespaces: connection.namespaces
                        .map((namespace) => namespace.name)
                        .filter(Boolean),
                    })),
                  })
                }}
              >
                <Plus size={14} />
                Add namespace
              </Button>
              <label className="checkbox-field">
                <input
                  type="checkbox"
                  checked={connection.operators.length > 0}
                  onChange={(event) => {
                    setConnection({
                      ...connection,
                      operators: event.target.checked
                        ? [
                            {
                              id: `operator-${suffix}`,
                              namespace: '',
                              deployment: '',
                              container: 'manager',
                              namespaces: connection.namespaces
                                .map((namespace) => namespace.name)
                                .filter(Boolean),
                            },
                          ]
                        : [],
                    })
                  }}
                />
                Observe the operator for these namespaces
              </label>
              {connection.operators.map((operator) => (
                <div key={operator.id} className="form-grid">
                  {(['namespace', 'deployment', 'container'] as const).map((field) => (
                    <Field
                      key={field}
                      label={
                        field === 'namespace'
                          ? 'Operator namespace'
                          : field === 'deployment'
                            ? 'Operator Deployment'
                            : 'Controller container'
                      }
                    >
                      <input
                        required
                        value={operator[field]}
                        onChange={(event) => {
                          setConnection({
                            ...connection,
                            operators: [{ ...operator, [field]: event.target.value }],
                          })
                        }}
                      />
                    </Field>
                  ))}
                </div>
              ))}
              <p className="onboarding-help">
                You can assign teams, environments, and additional operators after connecting.
              </p>
              <RenewalFields
                connection={connection}
                onChange={(next) => {
                  setConnection(next)
                  if (next.renewal && method === 'certificate') {
                    setMethod('token')
                    setCertificate('')
                    setKey('')
                  }
                }}
              />
            </>
          )}
          {step === 'Reader access' && (
            <OnboardingReader
              access={currentAccess}
              local={local}
              context={commandContext}
              onContext={setCommandContext}
              onNotice={setNotice}
              onError={setError}
            />
          )}
          {step === 'Credentials' && (
            <>
              <h2>{local ? 'Use the hosting cluster identity' : 'Add reader credentials'}</h2>
              {local ? (
                <p>
                  The console uses its mounted Kubernetes ServiceAccount token. Continue to check
                  its access to the enrolled namespaces.
                </p>
              ) : (
                <>
                  <Field label="Credential method">
                    <select
                      value={method}
                      onChange={(event) => {
                        const value = event.target.value
                        if (value === 'kubeconfig' || value === 'token' || value === 'certificate')
                          setMethod(value)
                      }}
                    >
                      <option value="kubeconfig">Import reader kubeconfig</option>
                      <option value="token">Bearer token</option>
                      <option value="certificate" disabled={!!connection.renewal}>
                        Client certificate
                      </option>
                    </select>
                  </Field>
                  {method === 'kubeconfig' ? (
                    <>
                      <Field label="Kubeconfig file">
                        <input
                          type="file"
                          accept=".yaml,.yml,.conf,.kubeconfig"
                          onChange={(event) => {
                            const file = event.target.files?.[0]
                            if (!file) return
                            if (file.size > 800_000) {
                              setError('Kubeconfig must be smaller than 800 KiB.')
                              return
                            }
                            void file
                              .text()
                              .then(setKubeconfig)
                              .catch(() => {
                                setError('The kubeconfig file could not be read.')
                              })
                          }}
                        />
                      </Field>
                      <Field
                        label="Reader kubeconfig"
                        help="Embedded token or certificate only. Authentication plugins and file references are rejected."
                      >
                        <textarea
                          required
                          autoComplete="off"
                          rows={8}
                          value={kubeconfig}
                          onChange={(event) => {
                            setKubeconfig(event.target.value)
                          }}
                        />
                      </Field>
                      <Field
                        label="Kubeconfig context"
                        help="Leave empty to use its current context."
                      >
                        <input
                          value={context}
                          onChange={(event) => {
                            setContext(event.target.value)
                          }}
                        />
                      </Field>
                    </>
                  ) : (
                    <>
                      <Field label="API server">
                        <input
                          required
                          type="url"
                          placeholder="https://kubernetes.example:6443"
                          value={connection.server}
                          onChange={(event) => {
                            setConnection({ ...connection, server: event.target.value })
                          }}
                        />
                      </Field>
                      <Field label="CA certificate" help="Leave empty to use system trust.">
                        <textarea
                          rows={3}
                          value={ca}
                          onChange={(event) => {
                            setCA(event.target.value)
                          }}
                        />
                      </Field>
                      {method === 'token' ? (
                        <Field label="Bearer token">
                          <input
                            required
                            type="password"
                            autoComplete="new-password"
                            value={token}
                            onChange={(event) => {
                              setToken(event.target.value)
                            }}
                          />
                        </Field>
                      ) : (
                        <div className="form-grid">
                          <Field label="Client certificate">
                            <textarea
                              required
                              rows={5}
                              value={certificate}
                              onChange={(event) => {
                                setCertificate(event.target.value)
                              }}
                            />
                          </Field>
                          <Field label="Private key">
                            <textarea
                              required
                              rows={5}
                              autoComplete="off"
                              value={key}
                              onChange={(event) => {
                                setKey(event.target.value)
                              }}
                            />
                          </Field>
                        </div>
                      )}
                    </>
                  )}
                </>
              )}
            </>
          )}
          {step === 'Verify' && (
            <>
              <h2>Verify {connection.name}</h2>
              <p className="onboarding-help">
                Checks reader access and previews deployments before saving this cluster.
              </p>
              <DetailRows
                rows={[
                  {
                    label: 'Namespaces',
                    value: connection.namespaces.map((namespace) => namespace.name).join(', '),
                  },
                  {
                    label: 'Reader',
                    value: local ? 'Hosting cluster ServiceAccount' : 'Uploaded reader credential',
                  },
                  ...(local
                    ? []
                    : [
                        {
                          label: 'Credential renewal',
                          value: connection.renewal
                            ? `Automatic, as ${connection.renewal.namespace}/${connection.renewal.serviceAccount}`
                            : 'Manual replacement',
                        },
                      ]),
                ]}
              />
              <div className="verify-action">
                <Button
                  type="button"
                  variant={currentCheck ? 'outline' : 'default'}
                  onClick={() => {
                    void test()
                  }}
                >
                  {pendingAction === 'check'
                    ? 'Checking…'
                    : currentCheck
                      ? 'Check again'
                      : 'Verify connection'}
                </Button>
                {!currentCheck && pendingAction !== 'check' && (
                  <span className="muted">Nothing is saved until you connect the cluster.</span>
                )}
              </div>
              {currentCheck && (
                <OnboardingChecks
                  result={currentCheck}
                  local={local}
                  renewal={!!connection.renewal}
                />
              )}
            </>
          )}
        </fieldset>
        {error && (
          <div className="form-error" role="alert">
            {error}
            {conflict && (
              <>
                <p>Reload the latest settings before saving. Your cluster details stay here.</p>
                <Button
                  type="button"
                  disabled={pending}
                  onClick={() => {
                    setPendingAction('reload')
                    void query
                      .refetch()
                      .then((result) => {
                        if (result.isError) setError(formError(result.error))
                        else {
                          setConflict(false)
                          setError('')
                          setNotice('Settings reloaded. Review the cluster and save again.')
                        }
                      })
                      .finally(() => {
                        setPendingAction(null)
                      })
                  }}
                >
                  Reload settings
                </Button>
              </>
            )}
          </div>
        )}
        {notice && (
          <p className="save-notice" role="status">
            {notice}
          </p>
        )}
        <div className="onboarding-actions">
          {stepIndex > 0 && (
            <Button
              type="button"
              variant="ghost"
              disabled={pending}
              onClick={() => {
                const previous = steps[stepIndex - 1]
                if (previous) {
                  setStep(previous)
                  setError('')
                  setNotice('')
                }
              }}
            >
              <ArrowLeft size={15} />
              Back
            </Button>
          )}
          {step !== 'Verify' ? (
            <Button type="submit" variant="default" disabled={pending}>
              {pendingAction === 'prepare' ? 'Preparing…' : 'Continue'}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                disabled={pending}
                onClick={() => {
                  void connect(true)
                }}
              >
                Save paused
              </Button>
              <Button
                type="button"
                variant="default"
                disabled={pending || currentCheck?.connection.collection !== 'current'}
                onClick={() => {
                  void connect(false)
                }}
              >
                {pendingAction === 'save' ? 'Saving…' : 'Connect cluster'}
              </Button>
            </>
          )}
        </div>
      </form>
    </div>
  )
}

function ConnectedSummary({
  id,
  name,
  paused,
  preview,
}: {
  id: string
  name: string
  paused: boolean
  preview: readonly Deployment[]
}) {
  const [waiting, setWaiting] = useState(!paused)
  // Poll quickly until the collector reports its first result for this cluster.
  const { data: snapshot } = useQuery({
    ...snapshotOptions,
    refetchInterval: waiting ? 3_000 : snapshotOptions.refetchInterval,
  })
  const observed = snapshot?.connections.find((item) => item.id === id)
  const collected = observed !== undefined && observed.collection !== 'pending'
  if (waiting && collected) setWaiting(false)
  const deployments = collected
    ? (snapshot?.deployments.filter((item) => item.connectionId === id) ?? [])
    : preview
  return (
    <div className="page-enter onboarding-layout">
      <PageHeading title={paused ? 'Cluster saved paused' : 'Cluster connected'} />
      <section className="onboarding-summary">
        <h2>{name}</h2>
        <ol className="progress-list" role="status">
          <li className="progress-done">
            <Check size={16} />
            Configuration saved
          </li>
          {paused ? (
            <li>
              <Pause size={16} />
              Collection paused. Verify reader credentials in Settings before enabling collection.
            </li>
          ) : !collected ? (
            <li className="progress-active">
              <Loader2 size={16} className="spin" />
              Waiting for first collection…
            </li>
          ) : observed.collection === 'current' ? (
            <li className="progress-done">
              <Check size={16} />
              First collection completed
            </li>
          ) : (
            <li className="progress-failed">
              <AlertCircle size={16} />
              First collection: {collectionLabels[observed.collection].toLocaleLowerCase()}. Review
              the reader in cluster settings.
            </li>
          )}
          {!paused && (
            <li className={collected ? 'progress-done' : ''}>
              {collected ? <Check size={16} /> : <CircleDashed size={16} />}
              {collected
                ? `${String(deployments.length)} ${deployments.length === 1 ? 'deployment' : 'deployments'} observed`
                : 'Deployment inventory'}
            </li>
          )}
        </ol>
        {deployments.length > 0 && (
          <section className="onboarding-preview">
            <h3>{collected ? 'Observed deployments' : 'Verification preview'}</h3>
            <ul className="onboarding-result-list">
              {deployments.map((deployment) => (
                <li key={deployment.id}>
                  <span>
                    <strong>{deployment.name}</strong>
                    <small>{deployment.namespace}</small>
                  </span>
                  {collected ? (
                    <HealthBadge health={deployment.health} />
                  ) : (
                    <EvidenceBadge collection="pending" />
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}
        <div className="onboarding-actions">
          <Button asChild variant="ghost">
            <Link to="/settings/clusters/new" reloadDocument>
              <Plus size={15} />
              Add another cluster
            </Link>
          </Button>
          <Button asChild variant={paused ? 'default' : 'outline'}>
            <Link to={paused ? '/settings' : '/clusters'}>
              {paused ? 'Open cluster settings' : 'View cluster'}
            </Link>
          </Button>
          {!paused && (
            <Button asChild variant="default">
              <Link to="/fleet" search={{ ...emptyFilter, cluster: id }}>
                View deployments
              </Link>
            </Button>
          )}
        </div>
      </section>
    </div>
  )
}
