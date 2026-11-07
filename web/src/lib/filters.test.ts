import { describe, expect, it } from 'vitest'
import { deploymentFixture, snapshotFixture } from '../test/fixtures'
import type { ClusterNames } from './filters'
import {
  clusterNames,
  collectionGaps,
  emptyFilter,
  filterDeployments,
  matchesQuery,
} from './filters'

const payments = deploymentFixture({
  id: 'payments',
  name: 'payments-bao',
  namespace: 'payments',
  team: 'Payments',
  connectionId: 'prod-eu-1',
  health: 'critical',
})
const staging = deploymentFixture({
  id: 'staging',
  name: 'staging-bao',
  namespace: 'staging',
  team: 'Platform',
  connectionId: 'staging-eu-1',
  environment: 'staging',
  evidence: { ...payments.evidence, collection: 'stale' },
})
const clusters: ClusterNames = new Map([['prod-eu-1', 'Frankfurt']])

describe('fleet query', () => {
  it('maps connection identifiers to names', () => {
    expect(clusterNames(snapshotFixture()).get('fixture-cluster')).toBe('Fixture cluster')
    expect(clusterNames(undefined).size).toBe(0)
  })
  it('matches the cluster name as well as its identifier', () => {
    expect(matchesQuery(payments, 'frankfurt', clusters)).toBe(true)
    expect(matchesQuery(payments, 'PROD-EU', clusters)).toBe(true)
    expect(matchesQuery(staging, 'frankfurt', clusters)).toBe(false)
  })
  it('matches name, namespace, and team, and ignores surrounding space', () => {
    expect(matchesQuery(payments, '  payments-bao ', clusters)).toBe(true)
    expect(matchesQuery(staging, 'platform', clusters)).toBe(true)
    expect(matchesQuery(staging, '', clusters)).toBe(true)
  })
})

describe('fleet filters', () => {
  const deployments = [payments, staging]
  it('combines filters', () => {
    expect(
      filterDeployments(deployments, { ...emptyFilter, health: 'critical' }, clusters),
    ).toEqual([payments])
    expect(
      filterDeployments(deployments, { ...emptyFilter, environment: 'staging' }, clusters),
    ).toEqual([staging])
    expect(
      filterDeployments(
        deployments,
        { ...emptyFilter, cluster: 'prod-eu-1', health: 'healthy' },
        clusters,
      ),
    ).toEqual([])
  })
  it('selects every deployment without current evidence for collection gaps', () => {
    expect(
      filterDeployments(deployments, { ...emptyFilter, collection: collectionGaps }, clusters),
    ).toEqual([staging])
  })
})
