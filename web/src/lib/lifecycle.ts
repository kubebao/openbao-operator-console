import type { Deployment } from '../api/schema'

export type UpgradeState = 'settled' | 'upgrading' | 'failed' | 'unknown'

// Upgrade state from public fields only: a recorded failure, the Upgrading condition, or a
// requested version that differs from the running version. Without both versions the
// upgrade cannot be assessed and stays unknown.
export function upgradeState(deployment: Deployment): UpgradeState {
  if (deployment.upgradeFailure) return 'failed'
  const upgrading = deployment.conditions.some(
    (condition) => condition.type === 'Upgrading' && condition.status === 'True',
  )
  if (upgrading) return 'upgrading'
  if (deployment.desiredVersion === '' || deployment.version === '') return 'unknown'
  return deployment.desiredVersion === deployment.version ? 'settled' : 'upgrading'
}

export const upgradeLabels: Record<UpgradeState, string> = {
  settled: 'Settled',
  upgrading: 'In progress',
  failed: 'Failed',
  unknown: 'Unknown',
}

// Short upgrade summaries for compact views.
export const upgradeSummaries: Record<UpgradeState, string> = {
  settled: 'Upgrade settled',
  upgrading: 'Upgrade in progress',
  failed: 'Upgrade failed',
  unknown: 'Upgrade state unknown',
}

// The newest resource generation that the operator has reported conditions for, or null
// when no conditions are reported. Conditions are the only public reconciliation signal.
export function observedGeneration(deployment: Deployment): number | null {
  if (deployment.conditions.length === 0) return null
  return Math.max(...deployment.conditions.map((condition) => condition.observedGeneration))
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
