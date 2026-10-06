import { describe, expect, it } from 'vitest'
import type { Deployment } from '../api/schema'
import { assessConditions } from './conditions'
import type { Observation } from './history'
import {
  backupEvents,
  conditionTransitions,
  describeChange,
  describeHistory,
  stateBand,
} from './history'

function deployment(overrides: Partial<Deployment> = {}): Deployment {
  return {
    id: 'fixture',
    uid: 'fixture-uid',
    resourceVersion: '1',
    generation: 3,
    name: 'fixture-bao',
    namespace: 'fixture',
    connectionId: 'cluster',
    location: '',
    team: 'Platform',
    environment: 'production',
    version: '2.7.0',
    desiredVersion: '2.7.0',
    operatorVersion: null,
    operatorId: null,
    role: 'primary',
    health: 'healthy',
    readyReplicas: 3,
    desiredReplicas: 3,
    evidence: {
      collection: 'current',
      collectedAt: '2026-10-06T12:00:00Z',
      lastAttemptAt: '2026-10-06T12:00:00Z',
      producerObservedAt: null,
      producerFreshness: 'unknown',
    },
    conditions: [
      {
        type: 'Available',
        status: 'True',
        reason: 'Ready',
        message: '',
        observedGeneration: 3,
        lastTransitionAt: null,
      },
    ],
    backup: {
      lastSuccessAt: '2026-10-05T12:00:00Z',
      nextAt: null,
      key: 'fixture/a.snapshot',
      digest: null,
      sizeBytes: 1024,
      consecutiveFailures: 0,
      restoreTest: null,
    },
    upgradeFailure: null,
    recovery: null,
    replica: null,
    issues: [],
    ...overrides,
  }
}

function backupOf(value: Deployment): NonNullable<Deployment['backup']> {
  if (!value.backup) throw new Error('fixture has no backup')
  return value.backup
}

function observation(at: string, value: Deployment): Observation {
  return { recordedAt: at, deployment: value }
}

describe('change descriptions', () => {
  it('describes state, replica, condition, and finding changes', () => {
    const before = deployment()
    const after = deployment({
      health: 'critical',
      readyReplicas: 0,
      conditions: [
        {
          type: 'OpenBaoSealed',
          status: 'True',
          reason: 'Sealed',
          message: 'Sealed.',
          observedGeneration: 3,
          lastTransitionAt: null,
        },
      ],
      issues: [
        {
          id: 'fixture:sealed',
          deploymentId: 'fixture',
          rule: 'sealed',
          severity: 'critical',
          category: 'availability',
          title: 'OpenBao is sealed',
          detail: '',
          firstSeenAt: '2026-10-06T12:00:00Z',
          lastCheckedAt: '2026-10-06T12:00:00Z',
        },
      ],
    })
    const titles = describeChange(before, after).map((change) => change.title)
    expect(titles).toEqual([
      'State Healthy → Critical',
      'Finding raised: OpenBao is sealed',
      'Ready replicas 3 → 0',
      'OpenBaoSealed reported True',
      'Available no longer reported',
    ])
  })

  it('reports a counter jump as observed failures, not individual attempts', () => {
    const before = deployment()
    const after = deployment({
      backup: { ...backupOf(before), consecutiveFailures: 2 },
    })
    expect(describeChange(before, after)).toEqual([
      { title: '2 backup failures observed', detail: '2 consecutive failures', tone: 'warning' },
    ])
  })

  it('keeps an entry for observations without a displayed change', () => {
    const entries = describeHistory([
      observation('2026-10-06T10:00:00Z', deployment({ resourceVersion: '2' })),
      observation('2026-10-06T09:00:00Z', deployment()),
    ])
    expect(entries.map((entry) => entry.changes[0]?.title)).toEqual([
      'Resource updated',
      'Earliest retained observation',
    ])
    expect(entries[1]?.earliest).toBe(true)
  })
})

describe('state band', () => {
  it('marks time before the oldest observation as unrecorded and merges equal states', () => {
    const start = Date.parse('2026-10-06T00:00:00Z')
    const end = Date.parse('2026-10-06T12:00:00Z')
    const segments = stateBand(
      [
        observation('2026-10-06T02:00:00Z', deployment()),
        observation('2026-10-06T04:00:00Z', deployment({ resourceVersion: '2' })),
        observation(
          '2026-10-06T06:00:00Z',
          deployment({
            evidence: { ...deployment().evidence, collection: 'unreachable' },
          }),
        ),
      ],
      start,
      end,
    )
    expect(segments.map((segment) => segment.state)).toEqual([
      'unrecorded',
      'healthy',
      'uncollected',
    ])
    expect(segments[1]?.end).toBe(Date.parse('2026-10-06T06:00:00Z'))
    expect(segments.at(-1)?.end).toBe(end)
  })
})

describe('backup events', () => {
  it('derives completions and observed failures', () => {
    const base = deployment()
    const events = backupEvents([
      observation('2026-10-06T08:00:00Z', base),
      observation(
        '2026-10-06T10:00:00Z',
        deployment({ backup: { ...backupOf(base), consecutiveFailures: 1 } }),
      ),
      observation(
        '2026-10-06T11:00:00Z',
        deployment({
          backup: { ...backupOf(base), lastSuccessAt: '2026-10-06T11:00:00Z', sizeBytes: 2048 },
        }),
      ),
    ])
    expect(events.map((event) => [event.kind, event.at])).toEqual([
      ['success', '2026-10-05T12:00:00Z'],
      ['failure', '2026-10-06T10:00:00Z'],
      ['success', '2026-10-06T11:00:00Z'],
    ])
  })
})

describe('conditions', () => {
  it('orders problems first and marks conditions from older generations', () => {
    const value = deployment({
      generation: 4,
      conditions: [
        {
          type: 'Available',
          status: 'True',
          reason: '',
          message: '',
          observedGeneration: 4,
          lastTransitionAt: null,
        },
        {
          type: 'TLSReady',
          status: 'False',
          reason: '',
          message: '',
          observedGeneration: 3,
          lastTransitionAt: null,
        },
      ],
    })
    const views = assessConditions(value)
    expect(views.map((view) => [view.condition.type, view.assessment, view.outdated])).toEqual([
      ['TLSReady', 'problem', true],
      ['Available', 'normal', false],
    ])
  })

  it('reads a finished activity as neutral unless its reason reports a failure', () => {
    const upgrading = (reason: string) =>
      assessConditions(
        deployment({
          conditions: [
            {
              type: 'Upgrading',
              status: 'False',
              reason,
              message: '',
              observedGeneration: 3,
              lastTransitionAt: null,
            },
          ],
        }),
      )[0]?.assessment
    expect(upgrading('UpgradeComplete')).toBe('informational')
    expect(upgrading('UpgradeFailed')).toBe('problem')
  })

  it('finds the latest status transition per condition', () => {
    const sealed = (status: 'True' | 'False') =>
      deployment({
        conditions: [
          {
            type: 'OpenBaoSealed',
            status,
            reason: '',
            message: '',
            observedGeneration: 3,
            lastTransitionAt: null,
          },
        ],
      })
    const transitions = conditionTransitions([
      observation('2026-10-06T09:00:00Z', sealed('False')),
      observation('2026-10-06T10:00:00Z', sealed('True')),
    ])
    expect(transitions.get('OpenBaoSealed')).toEqual({ from: 'False', at: '2026-10-06T10:00:00Z' })
  })
})
