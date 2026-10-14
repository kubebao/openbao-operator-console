import { describe, expect, it } from 'vitest'
import type { Configuration } from '../api/schema'
import { affectedBindings, issuerChanges } from './identity'

const configuration: Configuration = {
  connections: [],
  users: [
    { subject: '42', name: 'Ada', admin: true, scopes: [] },
    { subject: '7', name: '', admin: false, scopes: [] },
  ],
  groups: [{ name: 'platform', admin: false, scopes: [] }],
  relationships: [],
  maxBackupAgeHours: 24,
  startupGraceMinutes: 10,
}

describe('issuer changes', () => {
  it('compares issuers exactly', () => {
    expect(issuerChanges('https://identity.example', 'https://identity.example')).toBe(false)
    expect(issuerChanges('https://identity.example', 'https://identity.example/')).toBe(true)
    expect(issuerChanges('https://identity.example', 'https://gitlab.example')).toBe(true)
  })

  it('lists every user and group binding', () => {
    expect(affectedBindings(configuration)).toEqual([
      { kind: 'user', key: '42', label: 'Ada', detail: 'Administrator · subject 42' },
      { kind: 'user', key: '7', label: '7', detail: 'User · subject 7' },
      { kind: 'group', key: 'platform', label: 'platform', detail: 'Group' },
    ])
  })
})
