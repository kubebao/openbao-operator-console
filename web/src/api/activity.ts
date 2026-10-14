import { z } from 'zod'
import type { components } from './generated'

export type Activity = components['schemas']['Activity']
export type FindingRule = components['schemas']['FindingChange']['rule']

const timestamp = z.iso.datetime({ offset: true })
const health = z.enum(['healthy', 'attention', 'critical', 'unknown'])
const collection = z.enum(['current', 'stale', 'denied', 'unreachable', 'unsupported'])
const severity = z.enum(['critical', 'high', 'info'])

// The change object each kind carries. The others are null.
const changeOf = {
  healthChanged: 'health',
  collectionChanged: 'collection',
  findingRaised: 'finding',
  findingResolved: 'finding',
  findingSeverityChanged: 'finding',
  legacy: null,
} as const

export const activityObject = z
  .object({
    id: z.string(),
    deploymentId: z.string(),
    deploymentName: z.string(),
    connectionId: z.string(),
    namespace: z.string(),
    recordedAt: timestamp,
    kind: z.enum([
      'healthChanged',
      'collectionChanged',
      'findingRaised',
      'findingResolved',
      'findingSeverityChanged',
      'legacy',
    ]),
    health: z.object({ from: health, to: health }).nullable(),
    collection: z.object({ from: collection, to: collection }).nullable(),
    finding: z
      .object({
        rule: z.enum([
          'replica-readiness',
          'sealed',
          'upgrade-failed',
          'backup-failed',
          'backup-never-succeeded',
          'backup-overdue',
          'restore-test',
          'replica-history-conflict',
          'recovery-investigate',
          'collection',
        ]),
        severity,
        previousSeverity: severity.nullable(),
      })
      .nullable(),
  })
  .refine((event) => {
    const change = changeOf[event.kind]
    const severityChange = (event.finding?.previousSeverity ?? null) !== null
    return (
      (event.health !== null) === (change === 'health') &&
      (event.collection !== null) === (change === 'collection') &&
      (event.finding !== null) === (change === 'finding') &&
      severityChange === (event.kind === 'findingSeverityChanged')
    )
  }, 'The change does not match the event kind.')

export const activitySchema: z.ZodType<Activity> = activityObject

// ActivityContractSchema exposes the unannotated schema to the contract test.
export type ActivityContractSchema = typeof activityObject
