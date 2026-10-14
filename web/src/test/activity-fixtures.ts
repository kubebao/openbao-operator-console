import type { Activity } from '../api/activity'

// activityFixture is a fixture health transition. Overrides replace the change.
export function activityFixture(overrides: Partial<Activity> = {}): Activity {
  return {
    id: 'fixture-deployment:2026-10-06T12:00:00Z:healthChanged',
    deploymentId: 'fixture-deployment',
    deploymentName: 'fixture-bao',
    connectionId: 'fixture-cluster',
    namespace: 'fixture',
    recordedAt: '2026-10-06T12:00:00Z',
    kind: 'healthChanged',
    health: { from: 'healthy', to: 'critical' },
    collection: null,
    finding: null,
    ...overrides,
  }
}
