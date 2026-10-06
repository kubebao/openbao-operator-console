import type { Deployment } from '../api/schema'

export type UpgradeState = 'settled' | 'upgrading' | 'failed'

// Upgrade state from public fields only: a recorded failure, the Upgrading condition, or a
// requested version that differs from the running version.
export function upgradeState(deployment: Deployment): UpgradeState {
  if (deployment.upgradeFailure) return 'failed'
  const upgrading = deployment.conditions.some(
    (condition) => condition.type === 'Upgrading' && condition.status === 'True',
  )
  if (
    upgrading ||
    (deployment.desiredVersion !== '' &&
      deployment.version !== '' &&
      deployment.desiredVersion !== deployment.version)
  )
    return 'upgrading'
  return 'settled'
}

export const upgradeLabels: Record<UpgradeState, string> = {
  settled: 'Settled',
  upgrading: 'In progress',
  failed: 'Failed',
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
