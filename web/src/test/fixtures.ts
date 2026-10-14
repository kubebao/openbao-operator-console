import type { Deployment, Operator, Settings, Snapshot } from '../api/schema'

// Test fixtures. These values are illustrative and do not describe a real cluster.

export function deploymentFixture(overrides: Partial<Deployment> = {}): Deployment {
  return {
    id: 'fixture',
    uid: 'fixture-uid',
    resourceVersion: '1',
    generation: 3,
    name: 'fixture-bao',
    namespace: 'fixture',
    connectionId: 'fixture-cluster',
    location: '',
    team: 'Platform',
    environment: 'production',
    version: '2.7.0',
    desiredVersion: '2.7.0',
    operatorVersion: null,
    operatorId: 'fixture-operator',
    role: 'primary',
    health: 'healthy',
    readyReplicas: 3,
    desiredReplicas: 3,
    evidence: {
      collection: 'current',
      collectedAt: '2026-10-06T12:00:00Z',
      lastAttemptAt: '2026-10-06T12:00:00Z',
      producerObservedAt: null,
      producerFreshness: 'unknown',
    },
    conditions: [],
    backup: null,
    upgradeFailure: null,
    recovery: null,
    replica: null,
    issues: [],
    ...overrides,
  }
}

export function operatorFixture(overrides: Partial<Operator> = {}): Operator {
  return {
    id: 'fixture-operator',
    namespace: 'openbao-operator-system',
    name: 'openbao-operator-controller',
    container: 'manager',
    namespaces: ['fixture'],
    uid: 'fixture-operator-uid',
    image: 'example.invalid/openbao-operator:fixture',
    version: 'fixture',
    collection: 'current',
    lastAttemptAt: '2026-10-06T12:00:00Z',
    lastSuccessAt: '2026-10-06T12:00:00Z',
    health: 'available',
    generation: 1,
    observedGeneration: 1,
    readyReplicas: 1,
    desiredReplicas: 1,
    updatedReplicas: 1,
    ...overrides,
  }
}

export function snapshotFixture(overrides: Partial<Snapshot> = {}): Snapshot {
  return {
    mode: 'demo',
    generatedAt: '2026-10-06T12:00:00Z',
    build: { version: 'dev', commit: '' },
    deployments: [deploymentFixture()],
    connections: [
      {
        id: 'fixture-cluster',
        name: 'Fixture cluster',
        region: 'fixture-region',
        collection: 'current',
        lastSuccessAt: '2026-10-06T12:00:00Z',
        lastAttemptAt: '2026-10-06T12:00:00Z',
        namespaces: ['fixture'],
        capabilities: [],
        operators: [operatorFixture()],
      },
    ],
    relationships: [],
    viewer: { name: 'Fixture Viewer', subject: 'fixture-viewer', connectionIds: [], admin: true },
    policy: { maxBackupAgeHours: 24, startupGraceMinutes: 10 },
    ...overrides,
  }
}

export function settingsFixture(overrides: Partial<Settings['configuration']> = {}): Settings {
  return {
    revision: 1,
    configuration: {
      connections: [
        {
          id: 'fixture-cluster',
          name: 'Fixture cluster',
          region: '',
          disabled: false,
          authentication: 'local',
          server: '',
          credentialConfigured: false,
          namespaces: [
            { name: 'team-a', team: 'A', environment: 'production', location: '' },
            { name: 'team-b', team: 'B', environment: 'staging', location: '' },
          ],
          operators: [],
        },
      ],
      users: [],
      groups: [],
      relationships: [],
      maxBackupAgeHours: 24,
      startupGraceMinutes: 10,
      ...overrides,
    },
    identity: {
      issuer: 'https://identity.example',
      clientId: 'console',
      ca: '',
      groupsClaim: 'groups',
      secretConfigured: true,
    },
    changes: [],
  }
}
