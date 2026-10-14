import type { Configuration } from '../api/schema'

// AffectedBinding is a user or group whose access belongs to the active issuer.
export interface AffectedBinding {
  kind: 'user' | 'group'
  key: string
  label: string
  detail: string
}

// issuerChanges reports whether a candidate issuer differs from the active issuer. The
// comparison is exact because ID tokens must carry the exact configured issuer.
export function issuerChanges(activeIssuer: string, candidateIssuer: string): boolean {
  return activeIssuer !== candidateIssuer
}

// affectedBindings lists every user and group binding. All of them belong to the active
// issuer, so an issuer change affects each one.
export function affectedBindings(configuration: Configuration): AffectedBinding[] {
  return [
    ...configuration.users.map((user) => ({
      kind: 'user' as const,
      key: user.subject,
      label: user.name || user.subject,
      detail: `${user.admin ? 'Administrator' : 'User'} · subject ${user.subject}`,
    })),
    ...configuration.groups.map((group) => ({
      kind: 'group' as const,
      key: group.name,
      label: group.name,
      detail: group.admin ? 'Administrator group' : 'Group',
    })),
  ]
}
