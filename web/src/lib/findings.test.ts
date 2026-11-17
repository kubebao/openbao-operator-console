import { describe, expect, it } from 'vitest'
import type { Issue } from '../api/schema'
import {
  conditionSeverity,
  dimensionSeverity,
  fieldSeverity,
  findingLocation,
  tabSeverity,
} from './findings'

function issue(rule: string, severity: Issue['severity']): Issue {
  return {
    id: `fixture:${rule}`,
    deploymentId: 'fixture',
    rule,
    severity,
    category: 'availability',
    title: rule,
    detail: '',
    firstSeenAt: '2026-10-06T12:00:00Z',
    lastCheckedAt: '2026-10-06T12:00:00Z',
  }
}

describe('finding locations', () => {
  it('points backup findings to the backup evidence row', () => {
    expect(findingLocation(issue('backup-failed', 'high'))).toEqual({
      tab: 'backups',
      fields: ['backupFailures'],
    })
  })
  it('keeps unknown rules on the overview without claiming a field', () => {
    expect(findingLocation(issue('future-rule', 'high'))).toEqual({ tab: 'overview', fields: [] })
  })
  it('links operator findings to conditions and highlights only their source row', () => {
    const condition = { ...issue('operator-condition', 'high'), conditionType: 'TLSReady' }
    const deployment = {
      issues: [
        condition,
        { ...issue('availability', 'high'), conditionType: 'Available' },
        issue('sealed', 'critical'),
      ],
    }
    expect(findingLocation(condition).tab).toBe('conditions')
    expect(conditionSeverity(deployment, 'TLSReady')).toBe('high')
    expect(conditionSeverity(deployment, 'Available')).toBe('high')
    expect(conditionSeverity(deployment, 'OpenBaoSealed')).toBe('critical')
    expect(conditionSeverity(deployment, 'StorageConfigured')).toBeNull()
    expect(dimensionSeverity(deployment, 'availability')).toBe('critical')
  })
  it('reports the strongest severity per tab and field', () => {
    const deployment = {
      issues: [issue('replica-readiness', 'high'), issue('sealed', 'critical')],
    }
    expect(tabSeverity(deployment, 'conditions')).toBe('critical')
    expect(tabSeverity(deployment, 'overview')).toBe('high')
    expect(tabSeverity(deployment, 'backups')).toBeNull()
    expect(fieldSeverity(deployment, 'replicas')).toBe('high')
    expect(dimensionSeverity(deployment, 'availability')).toBe('critical')
    expect(dimensionSeverity(deployment, 'backups')).toBeNull()
  })
})
