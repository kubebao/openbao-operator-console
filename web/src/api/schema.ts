import { z } from 'zod'
import type { components } from './generated'

export type Snapshot = components['schemas']['Snapshot']
export type Deployment = components['schemas']['Deployment']
export type Issue = components['schemas']['Issue']
export type Relationship = components['schemas']['Relationship']
export type History = components['schemas']['History']
export type Operator = components['schemas']['Operator']

const timestamp = z.iso.datetime({ offset: true })
const collection = z.enum(['current', 'stale', 'denied', 'unreachable', 'unsupported'])
const condition = z.object({
  type: z.string(),
  status: z.enum(['True', 'False', 'Unknown']),
  reason: z.string(),
  message: z.string(),
  observedGeneration: z.number().int(),
  lastTransitionAt: timestamp.nullable(),
})
const restoreTest = z.object({
  outcome: z.enum(['notRun', 'running', 'passed', 'failed', 'blocked']),
  finishedAt: timestamp.nullable(),
  lastSuccessAt: timestamp.nullable(),
  key: z.string().nullable(),
  digest: z.string().nullable(),
  reason: z.string(),
})
export const deploymentSchema: z.ZodType<Deployment> = z.object({
  id: z.string(),
  uid: z.string(),
  resourceVersion: z.string(),
  generation: z.number().int(),
  name: z.string(),
  namespace: z.string(),
  connectionId: z.string(),
  location: z.string(),
  team: z.string(),
  environment: z.enum(['production', 'staging', 'development', 'other']),
  version: z.string(),
  desiredVersion: z.string(),
  operatorVersion: z.string().nullable(),
  operatorId: z.string().nullable(),
  role: z.enum(['primary', 'replica', 'recoveryTarget']),
  health: z.enum(['healthy', 'attention', 'critical', 'unknown']),
  readyReplicas: z.number().int().nonnegative().nullable(),
  desiredReplicas: z.number().int().nonnegative().nullable(),
  evidence: z.object({
    collection,
    collectedAt: timestamp,
    lastAttemptAt: timestamp,
    producerObservedAt: timestamp.nullable(),
    producerFreshness: z.enum(['unknown', 'fresh', 'stale']),
  }),
  conditions: z.array(condition),
  backup: z
    .object({
      lastSuccessAt: timestamp.nullable(),
      nextAt: timestamp.nullable(),
      key: z.string().nullable(),
      digest: z.string().nullable(),
      sizeBytes: z.number().int().nonnegative().nullable(),
      consecutiveFailures: z.number().int().nonnegative(),
      restoreTest: restoreTest.nullable(),
    })
    .nullable(),
  upgradeFailure: z.object({ reason: z.string(), at: timestamp.nullable() }).nullable(),
  recovery: z
    .object({
      requestName: z.string(),
      mechanism: z.enum(['snapshot', 'native']),
      phase: z.enum(['pending', 'validating', 'running', 'completed', 'failed', 'unknown']),
      stage: z.string(),
      startedAt: timestamp.nullable(),
      completedAt: timestamp.nullable(),
      applicationConfirmedAt: timestamp.nullable(),
      cleanup: z.enum(['notApplicable', 'pending', 'complete', 'failed']),
      exclusionAcknowledged: z.boolean().nullable(),
    })
    .nullable(),
  replica: z
    .object({
      observedAt: timestamp.nullable(),
      localIndex: z.string().regex(/^\d+$/).nullable(),
      sourceIndex: z.string().regex(/^\d+$/).nullable(),
      historyConflict: z.boolean(),
    })
    .nullable(),
  issues: z.array(
    z.object({
      id: z.string(),
      deploymentId: z.string(),
      rule: z.string(),
      severity: z.enum(['critical', 'high', 'info']),
      category: z.enum(['availability', 'operations', 'backup', 'visibility']),
      title: z.string(),
      detail: z.string(),
      firstSeenAt: timestamp,
      lastCheckedAt: timestamp,
    }),
  ),
})

const operatorSchema: z.ZodType<Operator> = z.object({
  id: z.string(),
  namespace: z.string(),
  name: z.string(),
  container: z.string(),
  namespaces: z.array(z.string()),
  uid: z.string().nullable(),
  image: z.string().nullable(),
  version: z.string().nullable(),
  collection: z.enum(['current', 'stale', 'denied', 'unreachable', 'notFound', 'invalid']),
  lastAttemptAt: timestamp,
  lastSuccessAt: timestamp.nullable(),
  health: z.enum(['available', 'progressing', 'unavailable', 'unknown']),
  generation: z.number().int(),
  observedGeneration: z.number().int().nullable(),
  readyReplicas: z.number().int().nonnegative().nullable(),
  desiredReplicas: z.number().int().nonnegative().nullable(),
  updatedReplicas: z.number().int().nonnegative().nullable(),
})

const snapshotObject = z.object({
  mode: z.enum(['demo', 'live']),
  collectedAt: timestamp,
  deployments: z.array(deploymentSchema),
  connections: z.array(
    z.object({
      id: z.string(),
      name: z.string(),
      region: z.string(),
      collection: z.enum(['current', 'stale', 'denied', 'unreachable', 'unsupported', 'pending']),
      lastSuccessAt: timestamp.nullable(),
      lastAttemptAt: timestamp,
      namespaces: z.array(z.string()),
      capabilities: z.array(
        z.object({
          namespace: z.string(),
          resource: z.enum(['openbaoclusters', 'openbaorestores', 'openbaorecoveries']),
          state: z.enum(['available', 'unsupported', 'denied', 'unreachable', 'notChecked']),
          checkedAt: timestamp.nullable(),
        }),
      ),
      operators: z.array(operatorSchema),
    }),
  ),
  relationships: z.array(
    z.object({
      id: z.string(),
      sourceId: z.string(),
      targetId: z.string(),
      kind: z.enum(['replication', 'snapshotRecovery']),
      repository: z.string().nullable(),
      provenance: z.enum(['declared', 'observed', 'historical']),
      observedAt: timestamp.nullable(),
    }),
  ),
  activity: z.array(
    z.object({
      id: z.string(),
      deploymentId: z.string(),
      at: timestamp,
      title: z.string(),
      detail: z.string(),
      outcome: z.enum(['success', 'warning', 'info', 'error']),
    }),
  ),
  viewer: z.object({
    name: z.string(),
    subject: z.string(),
    connectionIds: z.array(z.string()),
    admin: z.boolean(),
  }),
  policy: z.object({
    maxBackupAgeHours: z.number().int().positive(),
    startupGraceMinutes: z.number().int().nonnegative(),
  }),
})

export const snapshotSchema: z.ZodType<Snapshot> = snapshotObject

export const historySchema: z.ZodType<History> = z.object({
  observations: z.array(z.object({ recordedAt: timestamp, deployment: deploymentSchema })),
})

export type Settings = components['schemas']['Settings']
export type Configuration = components['schemas']['Configuration']
export type ManagedConnection = components['schemas']['ManagedConnection']
export type CredentialUpdate = components['schemas']['CredentialUpdate']
export type ManagedUser = components['schemas']['ManagedUser']
export type GroupBinding = components['schemas']['GroupBinding']
export type ManagedRelationship = components['schemas']['ManagedRelationship']
export type Scope = components['schemas']['Scope']
export type IdentityInput = components['schemas']['IdentityInput']
export type ConnectionCheck = components['schemas']['ConnectionCheck']
export type ConnectionCheckResult = components['schemas']['ConnectionCheckResult']
export type SetupStatus = components['schemas']['SetupStatus']
export type SetupUnlock = components['schemas']['SetupUnlock']
export type ReaderAccessResult = components['schemas']['ReaderAccessResult']
export type CredentialStatus = components['schemas']['CredentialStatus']

const scopeSchema = z.object({ connectionId: z.string(), namespaces: z.array(z.string()) })
const managedNamespaceSchema = z.object({
  name: z.string(),
  team: z.string(),
  environment: z.enum(['production', 'staging', 'development', 'other']),
  location: z.string(),
})
const managedOperatorSchema = z.object({
  id: z.string(),
  namespace: z.string(),
  deployment: z.string(),
  container: z.string(),
  namespaces: z.array(z.string()),
})
export const managedConnectionSchema: z.ZodType<ManagedConnection> = z
  .object({
    renewal: z.object({ namespace: z.string(), serviceAccount: z.string() }).nullable().optional(),
    id: z.string(),
    name: z.string(),
    region: z.string(),
    disabled: z.boolean(),
    authentication: z.enum(['local', 'remote']),
    server: z.string(),
    credentialConfigured: z.boolean(),
    namespaces: z.array(managedNamespaceSchema),
    operators: z.array(managedOperatorSchema),
  })
  .transform(({ renewal, ...connection }) => ({
    ...connection,
    ...(renewal === undefined ? {} : { renewal }),
  }))
const resourceRefSchema = z.object({
  connectionId: z.string(),
  namespace: z.string(),
  name: z.string(),
  uid: z.string(),
})
const configurationSchema: z.ZodType<Configuration> = z.object({
  connections: z.array(managedConnectionSchema),
  users: z.array(
    z.object({
      subject: z.string(),
      name: z.string(),
      admin: z.boolean(),
      scopes: z.array(scopeSchema),
    }),
  ),
  groups: z.array(z.object({ name: z.string(), admin: z.boolean(), scopes: z.array(scopeSchema) })),
  relationships: z.array(
    z.object({
      id: z.string(),
      kind: z.enum(['replication', 'snapshotRecovery']),
      source: resourceRefSchema,
      target: resourceRefSchema,
      repository: z.string(),
    }),
  ),
  maxBackupAgeHours: z.number().int().positive(),
  startupGraceMinutes: z.number().int().nonnegative(),
})
export const settingsSchema: z.ZodType<Settings> = z.object({
  revision: z.number().int().positive(),
  configuration: configurationSchema,
  identity: z.object({
    issuer: z.string(),
    clientId: z.string(),
    ca: z.string(),
    groupsClaim: z.string(),
    secretConfigured: z.boolean(),
  }),
  changes: z.array(
    z.object({ revision: z.number().int(), actor: z.string(), summary: z.string(), at: timestamp }),
  ),
})
export const connectionCheckSchema: z.ZodType<ConnectionCheckResult> = z.object({
  connection: snapshotObject.shape.connections.element,
  rbac: z.string(),
  deployments: z.array(snapshotObject.shape.deployments.element),
  diagnostics: z.object({
    state: z.enum([
      'available',
      'unreachable',
      'tlsError',
      'unauthorized',
      'forbidden',
      'unsupported',
      'unknown',
    ]),
    message: z.string(),
  }),
  credentialExpiry: z.object({
    expiresAt: timestamp.nullable(),
    source: z.enum(['certificate', 'declaredToken', 'unknown']),
  }),
})

export const setupStatusSchema: z.ZodType<SetupStatus> = z.object({
  state: z.enum(['required', 'unlocked', 'complete', 'expired']),
  callbackUrl: z.string(),
  sessionExpiresAt: timestamp.nullable(),
})
export const readerAccessSchema: z.ZodType<ReaderAccessResult> = z.object({
  rbac: z.string(),
  serviceAccount: z.string(),
  serviceAccountNamespace: z.string(),
})
export const loginRedirectSchema: z.ZodType<components['schemas']['LoginRedirect']> = z.object({
  url: z.url().refine((value) => value.startsWith('https://'), 'Login URL must use HTTPS.'),
})

export const settingsUpdateSchema: z.ZodType<components['schemas']['SettingsUpdate']> = z.object({
  revision: z.number().int().positive(),
  configuration: configurationSchema,
  credentials: z.array(
    z
      .object({
        connectionId: z.string(),
        credential: z
          .object({
            server: z.string(),
            ca: z.string(),
            token: z.string(),
            certificate: z.string(),
            key: z.string(),
          })
          .optional(),
        kubeconfig: z.string().optional(),
        context: z.string().optional(),
      })
      .transform(({ connectionId, credential, kubeconfig, context }) => ({
        connectionId,
        ...(credential === undefined ? {} : { credential }),
        ...(kubeconfig === undefined ? {} : { kubeconfig }),
        ...(context === undefined ? {} : { context }),
      })),
  ),
})

export const credentialStatusesSchema: z.ZodType<components['schemas']['CredentialStatuses']> =
  z.object({
    credentials: z.array(
      z.object({
        connectionId: z.string(),
        mode: z.enum(['local', 'manual', 'automatic']),
        state: z.enum([
          'ready',
          'pending',
          'healthy',
          'retrying',
          'permissionDenied',
          'reconnectRequired',
          'paused',
        ]),
        expiresAt: timestamp.nullable(),
        expirySource: z.enum(['unknown', 'declaredToken', 'certificate', 'tokenRequest']),
        lastAttemptAt: timestamp.nullable(),
        lastSuccessAt: timestamp.nullable(),
        nextAttemptAt: timestamp.nullable(),
      }),
    ),
  })
