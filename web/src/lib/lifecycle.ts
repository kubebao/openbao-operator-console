import type { Deployment } from '../api/schema'
import { assessCondition } from './conditions'

export type UpgradeState = 'settled' | 'upgrading' | 'failed' | 'unknown'

// Interpret upgrade status only from current collection and resource acknowledgement.
// A condition acknowledges only its own status, independently of status.observedGeneration.
export function upgradeState(deployment: Deployment): UpgradeState {
  if (!generationCurrent(deployment)) return 'unknown'
  const upgrading = deployment.conditions.find((condition) => condition.type === 'Upgrading')
  if (
    upgrading &&
    (assessCondition(upgrading, deployment.generation).outdated || upgrading.status === 'Unknown')
  )
    return 'unknown'
  if (deployment.upgradeFailure) return 'failed'
  if (upgrading?.status === 'True') return 'upgrading'
  if (deployment.desiredVersion === '' || deployment.version === '') return 'unknown'
  return deployment.desiredVersion === deployment.version ? 'settled' : 'upgrading'
}

export const upgradeLabels: Record<UpgradeState, string> = {
  settled: 'Versions match',
  upgrading: 'In progress',
  failed: 'Failed',
  unknown: 'Unknown',
}

// Short upgrade summaries for compact views.
export const upgradeSummaries: Record<UpgradeState, string> = {
  settled: 'Versions match',
  upgrading: 'Upgrade in progress',
  failed: 'Upgrade failed',
  unknown: 'Upgrade state unknown',
}

// Resource acknowledgement is projected directly from status.observedGeneration.
// Older retained observations have no value; condition maxima cannot fill it in.
export function observedGeneration(deployment: Deployment): number | null {
  return deployment.observedGeneration
}

export function generationCurrent(deployment: Deployment): boolean {
  return (
    deployment.evidence.collection === 'current' &&
    deployment.generation > 0 &&
    deployment.observedGeneration === deployment.generation
  )
}

// How many log entries a remote replica trails its source, from the two reported indices.
// Indices exceed the safe integer range, so they are compared as big integers.
export function replicaLag(deployment: Deployment): bigint | null {
  const local = deployment.replica?.localIndex
  const source = deployment.replica?.sourceIndex
  if (!local || !source) return null
  const lag = BigInt(source) - BigInt(local)
  return lag < 0n ? 0n : lag
}
