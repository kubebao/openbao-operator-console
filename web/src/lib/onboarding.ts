import type { ManagedConnection } from '../api/schema'
import type { CredentialDraft } from './credential'
import { credentialComplete } from './credential'

export const steps = ['Cluster', 'Scope', 'Reader access', 'Credentials', 'Verify'] as const
export type Step = (typeof steps)[number]

const identifier = /^[a-zA-Z0-9_-]{1,100}$/
const namespace = /^[a-z0-9](?:[-a-z0-9]*[a-z0-9])?$/

// clusterID derives a stable identifier from the display name. The random suffix keeps
// identifiers unique when names repeat.
export function clusterID(name: string, suffix: string): string {
  const slug = name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(0, 70)
  return `${slug || 'cluster'}-${suffix}`
}

// withOperatorScope assigns every enrolled namespace to the onboarding operator.
export function withOperatorScope(connection: ManagedConnection): ManagedConnection {
  const names = connection.namespaces.map((item) => item.name).filter(Boolean)
  return {
    ...connection,
    operators: connection.operators.map((operator) => ({ ...operator, namespaces: names })),
  }
}

// scopeKey identifies the input reader RBAC depends on. The API server and pause state
// do not change the generated grants.
export function scopeKey(connection: ManagedConnection): string {
  return JSON.stringify({ ...connection, server: '', disabled: false })
}

// stepProblem explains why the administrator cannot leave a step yet.
export function stepProblem(
  step: Step,
  connection: ManagedConnection,
  draft: CredentialDraft,
  enrolled: readonly string[],
): string | null {
  switch (step) {
    case 'Cluster':
      if (!connection.name.trim() || !identifier.test(connection.id))
        return 'Enter a cluster name and a valid identifier.'
      if (enrolled.includes(connection.id))
        return 'This identifier is already enrolled. Choose a different identifier.'
      return null
    case 'Scope': {
      const names = connection.namespaces.map((item) => item.name)
      const valid = names.every((name) => name.length <= 63 && namespace.test(name))
      return names.length && valid && new Set(names).size === names.length
        ? null
        : 'Enter at least one valid, unique Kubernetes namespace.'
    }
    case 'Credentials':
      return credentialComplete(connection, draft)
        ? null
        : 'Supply a complete reader credential before verification.'
    case 'Reader access':
    case 'Verify':
      return null
  }
}
