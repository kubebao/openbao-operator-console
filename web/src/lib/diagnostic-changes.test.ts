import { describe, expect, it } from 'vitest'
import { deploymentFixture } from '../test/fixtures'
import { describeChange } from './changes'

describe('retained diagnostic evidence changes', () => {
  it('records resource acknowledgement without using a condition maximum', () => {
    const before = deploymentFixture({ observedGeneration: null })
    const after = deploymentFixture()
    expect(describeChange(before, after)).toContainEqual({
      title: 'Resource acknowledgement updated',
      detail: 'Observed generation unknown → 3',
      tone: 'neutral',
    })
  })
  it('records changed causes without treating each evaluation time as a change', () => {
    const base = deploymentFixture()
    const gap = {
      rule: 'recovery-investigate',
      reasons: ['scope_incomplete' as const],
      collectionOutcome: 'denied' as const,
      checkedAt: '2026-10-06T12:00:00Z',
    }
    const before = {
      ...base,
      evidence: { ...base.evidence, analysisIncomplete: true, analysisGaps: [gap] },
    }
    const checked = {
      ...before,
      evidence: {
        ...before.evidence,
        analysisGaps: [{ ...gap, checkedAt: '2026-10-06T12:01:00Z' }],
      },
    }
    expect(describeChange(before, checked)).toEqual([])
    const changed = {
      ...before,
      evidence: {
        ...before.evidence,
        analysisGaps: [{ ...gap, collectionOutcome: 'timeout' as const }],
      },
    }
    expect(describeChange(before, changed)).toContainEqual({
      title: 'Recorded diagnostic coverage changed',
      tone: 'neutral',
    })
  })
})
