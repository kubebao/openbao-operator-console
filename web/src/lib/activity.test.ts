import { describe, expect, it } from 'vitest'
import { activitySchema } from '../api/activity'
import { activityFixture } from '../test/activity-fixtures'
import { describeActivity } from './activity'

const finding = { rule: 'sealed', severity: 'critical', previousSeverity: null } as const

describe('activity events', () => {
  it('keeps earlier records unknown without inferring a change', () => {
    const event = activityFixture({ kind: 'legacy', health: null })
    expect(activitySchema.safeParse(event).success).toBe(true)
    expect(describeActivity(event)).toMatchObject({
      title: 'Earlier activity recorded',
      tone: 'neutral',
    })
    expect(
      activitySchema.safeParse({ ...event, health: { from: 'healthy', to: 'critical' } }).success,
    ).toBe(false)
  })
  it('describe transitions with the history wording', () => {
    expect(describeActivity(activityFixture())).toEqual({
      title: 'State Healthy → Critical',
      tone: 'critical',
    })
    expect(
      describeActivity(
        activityFixture({
          kind: 'collectionChanged',
          health: null,
          collection: { from: 'current', to: 'unreachable' },
        }),
      ),
    ).toEqual({ title: 'Collection Collected → Unreachable', tone: 'warning' })
  })

  it('describe raised, resolved, and changed findings by rule', () => {
    const raised = activityFixture({ kind: 'findingRaised', health: null, finding })
    expect(describeActivity(raised)).toEqual({
      title: 'Finding raised: OpenBao is sealed',
      detail: 'Severity Critical',
      tone: 'critical',
    })
    const resolved = activityFixture({ kind: 'findingResolved', health: null, finding })
    expect(describeActivity(resolved)).toMatchObject({
      title: 'Finding resolved: OpenBao is sealed',
      tone: 'success',
    })
    const escalated = activityFixture({
      kind: 'findingSeverityChanged',
      health: null,
      finding: { rule: 'backup-failed', severity: 'critical', previousSeverity: 'high' },
    })
    expect(describeActivity(escalated)).toEqual({
      title: 'Finding escalated: Backup failed',
      detail: 'Severity High → Critical',
      tone: 'critical',
    })
  })

  it('rejects an event whose change does not match its kind', () => {
    expect(activitySchema.safeParse(activityFixture()).success).toBe(true)
    expect(activitySchema.safeParse(activityFixture({ kind: 'findingRaised' })).success).toBe(false)
    expect(
      activitySchema.safeParse(activityFixture({ collection: { from: 'current', to: 'stale' } }))
        .success,
    ).toBe(false)
    const severity = activityFixture({ kind: 'findingRaised', health: null, finding })
    expect(activitySchema.safeParse(severity).success).toBe(true)
    expect(
      activitySchema.safeParse({ ...severity, finding: { ...finding, previousSeverity: 'high' } })
        .success,
    ).toBe(false)
  })
  it('names the condition in new events and accepts older events without it', () => {
    const event = activityFixture({
      kind: 'findingResolved',
      health: null,
      finding: {
        rule: 'operator-condition',
        conditionType: 'TLSReady',
        severity: 'high',
        previousSeverity: null,
      },
    })
    expect(activitySchema.safeParse(event).success).toBe(true)
    expect(describeActivity(event).title).toBe('Finding resolved: Operator condition: TLSReady')
    expect(
      activitySchema.safeParse({
        ...event,
        finding: { rule: 'operator-condition', severity: 'high', previousSeverity: null },
      }).success,
    ).toBe(true)
  })
})
