import type { Configuration, ManagedRelationship, Scope } from '../api/schema'

// AccessGrant edits a user or group. The key is the subject or group name.
export interface AccessGrant {
  key: string
  name: string
  admin: boolean
  scopes: readonly Scope[]
}

export type GrantKind = 'user' | 'group'

// EditMode distinguishes a new entry from a change to an existing one. Adding a
// duplicate is left to server validation instead of replacing the existing entry.
export type EditMode = 'add' | 'edit'

function enrolled(configuration: Configuration, connectionId: string, namespace: string): boolean {
  return configuration.connections.some(
    (connection) =>
      connection.id === connectionId &&
      connection.namespaces.some((item) => item.name === namespace),
  )
}

// PrunedGrant is a user or group that loses namespaces the configuration no longer enrolls.
// Namespaces are written as "connection/namespace".
export interface PrunedGrant {
  kind: GrantKind
  key: string
  label: string
  namespaces: readonly string[]
}

export interface PrunedReferences {
  grants: readonly PrunedGrant[]
  relationships: readonly ManagedRelationship[]
}

// prunedReferences lists what pruneScopes removes, so the change can be confirmed first.
export function prunedReferences(configuration: Configuration): PrunedReferences {
  const lost = (scopes: readonly Scope[]) =>
    scopes.flatMap((scope) =>
      scope.namespaces
        .filter((namespace) => !enrolled(configuration, scope.connectionId, namespace))
        .map((namespace) => `${scope.connectionId}/${namespace}`),
    )
  const grants: PrunedGrant[] = [
    ...configuration.users.map((user) => ({
      kind: 'user' as const,
      key: user.subject,
      label: user.name || user.subject,
      namespaces: lost(user.scopes),
    })),
    ...configuration.groups.map((group) => ({
      kind: 'group' as const,
      key: group.name,
      label: group.name,
      namespaces: lost(group.scopes),
    })),
  ]
  return {
    grants: grants.filter((grant) => grant.namespaces.length > 0),
    relationships: configuration.relationships.filter(
      (relationship) => !relationshipEnrolled(configuration, relationship),
    ),
  }
}

export function hasPrunedReferences(references: PrunedReferences): boolean {
  return references.grants.length > 0 || references.relationships.length > 0
}

function relationshipEnrolled(
  configuration: Configuration,
  relationship: ManagedRelationship,
): boolean {
  return [relationship.source, relationship.target].every((resource) =>
    enrolled(configuration, resource.connectionId, resource.namespace),
  )
}

// pruneScopes removes access grants and relationships that reference namespaces the
// configuration no longer enrolls.
export function pruneScopes(configuration: Configuration): Configuration {
  const prune = (scopes: readonly Scope[]) =>
    scopes
      .map((scope) => ({
        ...scope,
        namespaces: scope.namespaces.filter((namespace) =>
          enrolled(configuration, scope.connectionId, namespace),
        ),
      }))
      .filter((scope) => scope.namespaces.length)
  return {
    ...configuration,
    users: configuration.users.map((user) => ({ ...user, scopes: prune(user.scopes) })),
    groups: configuration.groups.map((group) => ({ ...group, scopes: prune(group.scopes) })),
    relationships: configuration.relationships.filter((relationship) =>
      relationshipEnrolled(configuration, relationship),
    ),
  }
}

// withGrant adds or replaces a user or group.
export function withGrant(
  configuration: Configuration,
  kind: GrantKind,
  mode: EditMode,
  grant: AccessGrant,
): Configuration {
  if (kind === 'user') {
    const user = { subject: grant.key, name: grant.name, admin: grant.admin, scopes: grant.scopes }
    return {
      ...configuration,
      users:
        mode === 'edit'
          ? configuration.users.map((item) => (item.subject === grant.key ? user : item))
          : [...configuration.users, user],
    }
  }
  const group = { name: grant.key, admin: grant.admin, scopes: grant.scopes }
  return {
    ...configuration,
    groups:
      mode === 'edit'
        ? configuration.groups.map((item) => (item.name === grant.key ? group : item))
        : [...configuration.groups, group],
  }
}

// withRelationship adds or replaces a relationship.
export function withRelationship(
  configuration: Configuration,
  mode: EditMode,
  relationship: ManagedRelationship,
): Configuration {
  return {
    ...configuration,
    relationships:
      mode === 'edit'
        ? configuration.relationships.map((item) =>
            item.id === relationship.id ? relationship : item,
          )
        : [...configuration.relationships, relationship],
  }
}

// wholeNumber parses a typed policy value. It returns null for empty, fractional, or
// out-of-range input.
export function wholeNumber(value: string, min: number): number | null {
  const trimmed = value.trim()
  if (!/^\d+$/u.test(trimmed)) return null
  const parsed = Number(trimmed)
  return Number.isSafeInteger(parsed) && parsed >= min ? parsed : null
}

export function scopedNamespaceCount(scopes: readonly Scope[]): number {
  return scopes.reduce((count, scope) => count + scope.namespaces.length, 0)
}

export function toggleConnection(configuration: Configuration, id: string): Configuration {
  return {
    ...configuration,
    connections: configuration.connections.map((connection) =>
      connection.id === id ? { ...connection, disabled: !connection.disabled } : connection,
    ),
  }
}

export function withoutConnection(configuration: Configuration, id: string): Configuration {
  return {
    ...configuration,
    connections: configuration.connections.filter((connection) => connection.id !== id),
  }
}

export function withoutGrant(
  configuration: Configuration,
  kind: GrantKind,
  key: string,
): Configuration {
  return kind === 'user'
    ? { ...configuration, users: configuration.users.filter((user) => user.subject !== key) }
    : { ...configuration, groups: configuration.groups.filter((group) => group.name !== key) }
}

export function withoutRelationship(configuration: Configuration, id: string): Configuration {
  return {
    ...configuration,
    relationships: configuration.relationships.filter((relationship) => relationship.id !== id),
  }
}
