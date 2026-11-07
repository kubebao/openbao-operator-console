import { describe, expect, it } from 'vitest'
import { deploymentFixture, operatorFixture, snapshotFixture } from '../test/fixtures'
import type { Box } from './topology'
import {
  groupCapabilities,
  linkedDeployments,
  operatorGroups,
  roundedPath,
  routeEdge,
  visibleOnMap,
} from './topology'

const connection = snapshotFixture().connections[0]
if (!connection) throw new Error('fixture has no connection')

describe('operator groups', () => {
  it('assigns deployments only to the operator enrollment declares', () => {
    const managed = deploymentFixture({ id: 'managed', name: 'b-bao' })
    const unmanaged = deploymentFixture({ id: 'unmanaged', operatorId: null })
    const elsewhere = deploymentFixture({ id: 'elsewhere', connectionId: 'other-cluster' })
    const groups = operatorGroups(connection, [managed, unmanaged, elsewhere])
    expect(groups.map((group) => group.operator?.id ?? null)).toEqual(['fixture-operator', null])
    expect(groups[0]?.deployments.map((item) => item.id)).toEqual(['managed'])
    expect(groups[1]?.deployments.map((item) => item.id)).toEqual(['unmanaged'])
  })
  it('does not trust an operator identifier that is not enrolled on the cluster', () => {
    const foreign = deploymentFixture({ id: 'foreign', operatorId: 'operator-elsewhere' })
    const groups = operatorGroups(connection, [foreign])
    expect(groups.at(-1)?.operator).toBeNull()
    expect(groups.at(-1)?.deployments.map((item) => item.id)).toEqual(['foreign'])
  })
  it('keeps an enrolled operator without deployments and orders by triage', () => {
    const groups = operatorGroups(
      { ...connection, operators: [operatorFixture(), operatorFixture({ id: 'idle' })] },
      [
        deploymentFixture({ id: 'a', name: 'a-bao', health: 'healthy' }),
        deploymentFixture({ id: 'b', name: 'b-bao', health: 'critical' }),
      ],
    )
    expect(groups[0]?.deployments.map((item) => item.id)).toEqual(['b', 'a'])
    expect(groups[1]).toEqual({ operator: operatorFixture({ id: 'idle' }), deployments: [] })
  })
})

describe('map visibility', () => {
  const many = Array.from({ length: 10 }, (_, index) =>
    deploymentFixture({ id: `d${String(index)}` }),
  )
  it('shows every deployment up to the limit', () => {
    expect(visibleOnMap(many.slice(0, 8), new Set(), 8)).toEqual({
      visible: many.slice(0, 8),
      hidden: [],
    })
  })
  it('keeps relationship endpoints visible above the limit', () => {
    const linked = linkedDeployments([
      {
        id: 'r',
        kind: 'replication',
        sourceId: 'd3',
        targetId: 'remote',
        provenance: 'declared',
        repository: null,
        observedAt: null,
      },
    ])
    const result = visibleOnMap(many, linked, 8)
    expect(result.visible.map((item) => item.id)).toEqual(['d3'])
    expect(result.hidden).toHaveLength(9)
  })
})

describe('resource API grouping', () => {
  const checkedAt = '2026-10-06T12:00:00Z'
  it('writes one row per result and namespace set', () => {
    expect(
      groupCapabilities([
        { namespace: 'a', resource: 'openbaoclusters', state: 'available', checkedAt },
        { namespace: 'b', resource: 'openbaoclusters', state: 'available', checkedAt },
        { namespace: 'a', resource: 'openbaorestores', state: 'available', checkedAt },
        { namespace: 'b', resource: 'openbaorestores', state: 'available', checkedAt },
        { namespace: 'a', resource: 'openbaorecoveries', state: 'unsupported', checkedAt },
      ]),
    ).toEqual([
      {
        resources: ['openbaoclusters', 'openbaorestores'],
        state: 'available',
        namespaces: ['a', 'b'],
      },
      { resources: ['openbaorecoveries'], state: 'unsupported', namespaces: ['a'] },
    ])
  })
  it('keeps APIs apart when their namespaces differ', () => {
    expect(
      groupCapabilities([
        { namespace: 'a', resource: 'openbaoclusters', state: 'denied', checkedAt },
        { namespace: 'b', resource: 'openbaorestores', state: 'denied', checkedAt },
      ]),
    ).toHaveLength(2)
  })
})

describe('edge routing', () => {
  const card = (column: number, row: number): Box => ({
    left: column * 300,
    right: column * 300 + 240,
    top: row * 300,
    bottom: row * 300 + 200,
  })
  const gap = { column: 60, row: 40 }
  // Every vertical segment runs in a gap: not strictly inside any card's horizontal span.
  const inGaps = (points: readonly (readonly [number, number])[], cards: Box[]) =>
    points.slice(1, -1).every(([x]) => cards.every((item) => x <= item.left || x >= item.right))

  it('joins neighbouring cards through the gap between them', () => {
    const route = routeEdge({ card: card(0, 0), y: 50 }, { card: card(1, 0), y: 80 }, gap)
    expect(route.points).toEqual([
      [240, 50],
      [270, 50],
      [270, 80],
      [300, 80],
    ])
  })
  it('runs above the row to reach a card that is not a neighbour', () => {
    const cards = [card(0, 0), card(1, 0), card(2, 0)]
    const route = routeEdge({ card: card(0, 0), y: 50 }, { card: card(2, 0), y: 80 }, gap)
    expect(route.points).toHaveLength(6)
    expect(route.points[2]?.[1]).toBe(-20)
    expect(inGaps(route.points, cards)).toBe(true)
    expect(route.ports).toEqual([
      [240, 50],
      [600, 80],
    ])
  })
  it('loops through the right gap between two deployments of one cluster', () => {
    const route = routeEdge({ card: card(0, 0), y: 50 }, { card: card(0, 0), y: 90 }, gap)
    expect(route.points).toEqual([
      [240, 50],
      [270, 50],
      [270, 90],
      [240, 90],
    ])
  })
  it('enters a card to the left from its right side', () => {
    const route = routeEdge({ card: card(2, 1), y: 350 }, { card: card(0, 0), y: 50 }, gap)
    expect(route.ports).toEqual([
      [600, 350],
      [240, 50],
    ])
    expect(inGaps(route.points, [card(0, 0), card(1, 0), card(2, 1)])).toBe(true)
  })
})

describe('rounded paths', () => {
  it('rounds inner corners and ends on the last point', () => {
    expect(
      roundedPath([
        [0, 0],
        [20, 0],
        [20, 20],
      ]),
    ).toBe('M 0 0 L 12 0 Q 20 0 20 8 L 20 20')
  })
})
