import { describe, expect, it } from 'vitest'
import { deploymentFixture, settingsFixture, snapshotFixture } from '../test/fixtures'
import {
  historySchema,
  loginRedirectSchema,
  managedConnectionSchema,
  settingsSchema,
  snapshotSchema,
} from './schema'

describe('snapshot schema', () => {
  it('parses a valid snapshot', () => {
    const snapshot = snapshotFixture()
    expect(snapshotSchema.parse(snapshot)).toEqual(snapshot)
  })
  it('rejects an unknown enum member instead of passing it through', () => {
    const deployment = { ...deploymentFixture(), health: 'fine' }
    expect(
      snapshotSchema.safeParse({ ...snapshotFixture(), deployments: [deployment] }).success,
    ).toBe(false)
  })
  it('rejects timestamps without an offset and missing fields', () => {
    expect(
      snapshotSchema.safeParse({ ...snapshotFixture(), generatedAt: '2026-10-06 12:00' }).success,
    ).toBe(false)
    const { viewer: _viewer, ...withoutViewer } = snapshotFixture()
    expect(snapshotSchema.safeParse(withoutViewer).success).toBe(false)
  })
  it('rejects replica indices that are not decimal strings', () => {
    const replica = {
      observedAt: null,
      localIndex: '1e3',
      sourceIndex: '5',
      historyConflict: false,
    }
    const history = {
      baseline: null,
      nextCursor: null,
      observations: [
        { recordedAt: '2026-10-06T12:00:00Z', deployment: { ...deploymentFixture(), replica } },
      ],
    }
    expect(historySchema.safeParse(history).success).toBe(false)
  })
})

describe('settings schema', () => {
  it('parses valid settings', () => {
    const settings = settingsFixture()
    expect(settingsSchema.parse(settings)).toEqual(settings)
  })
  it('rejects a non-positive revision and a negative grace period', () => {
    expect(settingsSchema.safeParse({ ...settingsFixture(), revision: 0 }).success).toBe(false)
    expect(settingsSchema.safeParse(settingsFixture({ startupGraceMinutes: -1 })).success).toBe(
      false,
    )
  })
  it('omits an absent renewal instead of reporting it as undefined', () => {
    const connection = settingsFixture().configuration.connections[0]
    expect(connection).toBeDefined()
    expect(Object.hasOwn(managedConnectionSchema.parse(connection), 'renewal')).toBe(false)
  })
})

describe('login redirect schema', () => {
  it('accepts only HTTPS login URLs', () => {
    expect(loginRedirectSchema.safeParse({ url: 'https://identity.example/auth' }).success).toBe(
      true,
    )
    expect(loginRedirectSchema.safeParse({ url: 'http://identity.example/auth' }).success).toBe(
      false,
    )
    expect(loginRedirectSchema.safeParse({ url: 'javascript:alert(1)' }).success).toBe(false)
  })
})
