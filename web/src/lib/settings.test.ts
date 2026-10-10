import { describe, expect, it } from 'vitest'
import type { Configuration } from '../api/schema'
import { newConnection } from './connection'
import { pruneScopes, withGrant, withRelationship } from './settings'

const configuration: Configuration = {
  connections: [
    {
      ...newConnection(),
      id: 'cluster',
      namespaces: [{ name: 'team-a', team: '', environment: 'other', location: '' }],
    },
  ],
  users: [
    {
      subject: 'engineer',
      name: 'Engineer',
      admin: false,
      scopes: [{ connectionId: 'cluster', namespaces: ['team-a', 'removed'] }],
    },
  ],
  groups: [
    { name: 'gone', admin: false, scopes: [{ connectionId: 'removed', namespaces: ['team-a'] }] },
  ],
  relationships: [
    {
      id: 'route',
      kind: 'replication',
      repository: '',
      source: { connectionId: 'cluster', namespace: 'team-a', name: 'a', uid: 'a' },
      target: { connectionId: 'cluster', namespace: 'removed', name: 'b', uid: 'b' },
    },
  ],
  maxBackupAgeHours: 24,
  startupGraceMinutes: 10,
}

describe('pruneScopes', () => {
  it('drops grants and relationships to unenrolled namespaces', () => {
    const pruned = pruneScopes(configuration)
    expect(pruned.users[0]?.scopes).toEqual([{ connectionId: 'cluster', namespaces: ['team-a'] }])
    expect(pruned.groups[0]?.scopes).toEqual([])
    expect(pruned.relationships).toEqual([])
  })
})

describe('withGrant', () => {
  it('replaces an existing user and appends a new group', () => {
    const grant = { key: 'engineer', name: 'Renamed', admin: true, scopes: [] }
    expect(withGrant(configuration, 'user', 'edit', grant).users).toEqual([
      { subject: 'engineer', name: 'Renamed', admin: true, scopes: [] },
    ])
    expect(
      withGrant(configuration, 'group', 'add', { ...grant, key: 'admins' }).groups,
    ).toHaveLength(2)
  })
})

describe('withRelationship', () => {
  it('replaces by identifier', () => {
    const relationship = configuration.relationships[0]
    if (!relationship) throw new Error('missing fixture')
    const next = withRelationship(configuration, 'edit', {
      ...relationship,
      repository: 's3://bucket',
    })
    expect(next.relationships).toHaveLength(1)
    expect(next.relationships[0]?.repository).toBe('s3://bucket')
  })
})
