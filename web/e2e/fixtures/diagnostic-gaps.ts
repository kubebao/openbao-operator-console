import type { Page } from '@playwright/test'
import { snapshotSchema } from '../../src/api/schema'

// Synthetic evidence illustrates independent resource and condition acknowledgement.
export async function diagnosticGaps(page: Page, observedGeneration: number | null = 3) {
  await page.route('**/api/v1/snapshot', async (route) => {
    const response = await route.fetch()
    const snapshot = snapshotSchema.parse(await response.json())
    const deployments = snapshot.deployments.map((deployment) => {
      if (deployment.id !== 'customer-x') return deployment
      return {
        ...deployment,
        generation: 4,
        observedGeneration,
        health: 'unknown' as const,
        issues: [],
        conditions: [
          {
            type: 'Available',
            status: 'True' as const,
            observedGeneration: 4,
            reason: 'Ready',
            message: '',
            lastTransitionAt: null,
          },
          {
            type: 'OpenBaoSealed',
            status: 'False' as const,
            observedGeneration: 3,
            reason: 'Unsealed',
            message: '',
            lastTransitionAt: null,
          },
          {
            type: 'Upgrading',
            status: 'True' as const,
            observedGeneration: 3,
            reason: 'Rolling',
            message: '',
            lastTransitionAt: null,
          },
        ],
        evidence: {
          ...deployment.evidence,
          analysisIncomplete: true,
          analysisGaps: [
            {
              rule: 'upgrade-failed',
              reasons: ['generation_unconfirmed' as const],
              collectionOutcome: 'complete' as const,
              checkedAt: snapshot.generatedAt,
            },
            {
              rule: 'recovery-investigate',
              reasons: ['scope_incomplete' as const],
              collectionOutcome: 'denied' as const,
              checkedAt: snapshot.generatedAt,
            },
          ],
        },
      }
    })
    await route.fulfill({ response, json: { ...snapshot, deployments } })
  })
}
