import type { ManagedConnection } from '../api/schema'

export type ManagedNamespace = ManagedConnection['namespaces'][number]
export type ManagedOperator = ManagedConnection['operators'][number]
export type Environment = ManagedNamespace['environment']
export type APIGroup = NonNullable<ManagedConnection['apiGroup']>

export function isAPIGroup(value: string): value is APIGroup {
  return value === 'openbao.org' || value === 'kubebao.org'
}

export const environments = [
  'production',
  'staging',
  'development',
  'other',
] as const satisfies readonly Environment[]

// Matches the server's connection identifier rule.
// Controller location for an OpenBao Operator chart installed as the release
// `openbao-operator` in `openbao-operator-system`, as the operator documentation does.
// Onboarding offers it as an editable starting point; verification checks the Deployment.
export const defaultOperatorLocation = {
  namespace: 'openbao-operator-system',
  deployment: 'openbao-operator-controller',
  container: 'manager',
} as const

export const connectionIDPattern = '[a-zA-Z0-9_-]{1,100}'
export const namespacePattern = '[a-z0-9]([-a-z0-9]*[a-z0-9])?'

export function isEnvironment(value: string): value is Environment {
  return environments.some((environment) => environment === value)
}

function newNamespace(): ManagedNamespace {
  return { name: '', team: '', environment: 'other', location: '' }
}

export function newConnection(): ManagedConnection {
  return {
    id: '',
    apiGroup: 'kubebao.org',
    name: '',
    region: '',
    authentication: 'remote',
    server: '',
    credentialConfigured: false,
    disabled: false,
    namespaces: [newNamespace()],
    operators: [],
  }
}

export function namespaceNames(connection: ManagedConnection): string[] {
  return connection.namespaces.map((namespace) => namespace.name).filter(Boolean)
}

export function addNamespace(connection: ManagedConnection): ManagedConnection {
  return { ...connection, namespaces: [...connection.namespaces, newNamespace()] }
}

// updateNamespace edits one namespace. A rename keeps the namespace assigned to the same
// operators.
export function updateNamespace(
  connection: ManagedConnection,
  index: number,
  change: Partial<ManagedNamespace>,
): ManagedConnection {
  const previous = connection.namespaces[index]?.name
  const renamed = change.name !== undefined && change.name !== previous
  return {
    ...connection,
    namespaces: connection.namespaces.map((namespace, position) =>
      position === index ? { ...namespace, ...change } : namespace,
    ),
    operators: renamed
      ? connection.operators.map((operator) => ({
          ...operator,
          namespaces: operator.namespaces.map((name) =>
            name === previous ? (change.name ?? name) : name,
          ),
        }))
      : connection.operators,
  }
}

export function removeNamespace(connection: ManagedConnection, index: number): ManagedConnection {
  const removed = connection.namespaces[index]?.name
  return {
    ...connection,
    namespaces: connection.namespaces.filter((_, position) => position !== index),
    operators: connection.operators.map((operator) => ({
      ...operator,
      namespaces: operator.namespaces.filter((name) => name !== removed),
    })),
  }
}

export function updateOperator(
  connection: ManagedConnection,
  index: number,
  change: Partial<ManagedOperator>,
): ManagedConnection {
  return {
    ...connection,
    operators: connection.operators.map((operator, position) =>
      position === index ? { ...operator, ...change } : operator,
    ),
  }
}

export function assignNamespace(
  connection: ManagedConnection,
  index: number,
  namespace: string,
  assigned: boolean,
): ManagedConnection {
  const operator = connection.operators[index]
  if (!operator) return connection
  const namespaces = operator.namespaces.filter((name) => name !== namespace)
  return updateOperator(connection, index, {
    namespaces: assigned ? [...namespaces, namespace] : namespaces,
  })
}

// withAuthentication switches between the hosting cluster and a remote reader. Only a
// remote reader can renew its own token.
export function withAuthentication(
  connection: ManagedConnection,
  authentication: ManagedConnection['authentication'],
): ManagedConnection {
  return { ...connection, authentication, ...(authentication === 'local' ? { renewal: null } : {}) }
}
