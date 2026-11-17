import type { Page } from '@playwright/test'
import type { Issue } from '../../src/api/schema'
import { snapshotSchema } from '../../src/api/schema'

// Illustrates projected condition findings without reading or changing Kubernetes.
export async function conditionFindings(page: Page) {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    const deployments = snapshot.deployments.map((deployment) => {
      if (deployment.id !== 'customer-x') return deployment
      const issues: readonly Issue[] = [
        {
          id: `${deployment.id}:availability`,
          deploymentId: deployment.id,
          rule: 'availability',
          conditionType: 'Available',
          severity: 'high',
          category: 'availability',
          title: 'Operator reports unavailable',
          detail: 'The operator reports Available=False for the current generation.',
          firstSeenAt: snapshot.generatedAt,
          lastCheckedAt: snapshot.generatedAt,
        },
        {
          id: `${deployment.id}:operator-condition:tls-ready`,
          deploymentId: deployment.id,
          rule: 'operator-condition',
          conditionType: 'TLSReady',
          severity: 'high',
          category: 'availability',
          title: 'Operator condition: TLSReady',
          detail: 'The operator reports TLSReady=False for the current generation.',
          firstSeenAt: snapshot.generatedAt,
          lastCheckedAt: snapshot.generatedAt,
        },
      ]
      const conditions = [
        ...deployment.conditions.filter(
          (condition) =>
            condition.type !== 'Available' &&
            condition.type !== 'TLSReady' &&
            condition.type !== 'StorageConfigured',
        ),
        ...['Available', 'TLSReady'].map((type) => ({
          type,
          status: 'False' as const,
          observedGeneration: deployment.generation,
          reason: 'Fixture',
          message: '',
          lastTransitionAt: snapshot.generatedAt,
        })),
        {
          type: 'StorageConfigured',
          status: 'True' as const,
          observedGeneration: deployment.generation,
          reason: 'Fixture',
          message: '',
          lastTransitionAt: snapshot.generatedAt,
        },
      ]
      return { ...deployment, health: 'attention' as const, conditions, issues }
    })
    await route.fulfill({ response, json: { ...snapshot, deployments } })
  })
}
