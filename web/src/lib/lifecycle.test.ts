import { describe, expect, it } from 'vitest'
import { deploymentFixture } from '../test/fixtures'
import { observedGeneration, replicaLag, upgradeState } from './lifecycle'

const upgrading = {
  type: 'Upgrading',
  status: 'True',
  reason: 'Rolling',
  message: '',
  observedGeneration: 3,
  lastTransitionAt: null,
} as const

describe('upgrade state', () => {
  it('is settled when the running version matches the requested version', () => {
    expect(upgradeState(deploymentFixture())).toBe('settled')
  })
  it('is in progress when the versions differ or the operator reports Upgrading', () => {
    expect(upgradeState(deploymentFixture({ desiredVersion: '2.8.0' }))).toBe('upgrading')
    expect(upgradeState(deploymentFixture({ conditions: [upgrading] }))).toBe('upgrading')
  })
  it('reports a recorded failure before any other evidence', () => {
    const failure = { reason: 'PostUpgradeCheckFailed', at: null }
    expect(
      upgradeState(deploymentFixture({ upgradeFailure: failure, conditions: [upgrading] })),
    ).toBe('failed')
  })
  it('stays unknown when either version is not observed', () => {
    expect(upgradeState(deploymentFixture({ version: '' }))).toBe('unknown')
    expect(upgradeState(deploymentFixture({ desiredVersion: '' }))).toBe('unknown')
    expect(upgradeState(deploymentFixture({ version: '', desiredVersion: '' }))).toBe('unknown')
  })
})

describe('reconciliation and replication evidence', () => {
  it('reports no generation without conditions', () => {
    expect(observedGeneration(deploymentFixture())).toBeNull()
    expect(observedGeneration(deploymentFixture({ conditions: [upgrading] }))).toBe(3)
  })
  it('compares replica indices beyond the safe integer range', () => {
    const replica = {
      observedAt: null,
      localIndex: '9007199254740993',
      sourceIndex: '9007199254741007',
      historyConflict: false,
    }
    expect(replicaLag(deploymentFixture({ replica }))).toBe(14n)
    expect(replicaLag(deploymentFixture({ replica: { ...replica, sourceIndex: null } }))).toBeNull()
  })
})
