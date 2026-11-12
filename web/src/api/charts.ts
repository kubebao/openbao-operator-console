import { z } from 'zod'
import type { components } from './generated'

export type ChartHistory = components['schemas']['ChartHistory']
export type ChartObservation = components['schemas']['ChartObservation']

const timestamp = z.iso.datetime({ offset: true })
const collection = z.enum(['current', 'stale', 'denied', 'unreachable', 'unsupported'])

const chartObservationObject = z.object({
  recordedAt: timestamp,
  deployment: z.object({
    health: z.enum(['healthy', 'attention', 'critical', 'unknown']),
    evidence: z.object({ collection }),
    backup: z
      .object({
        lastSuccessAt: timestamp.nullable(),
        consecutiveFailures: z.number().int().nonnegative().nullable(),
        sizeBytes: z.number().int().nonnegative().nullable(),
      })
      .nullable(),
  }),
})
const chartHistoryObject = z
  .object({
    start: timestamp,
    end: timestamp,
    observations: z.array(chartObservationObject),
    baseline: chartObservationObject.nullable(),
  })
  .refine(
    (data) =>
      Date.parse(data.start) < Date.parse(data.end) &&
      data.observations.every(
        (point) =>
          Date.parse(point.recordedAt) >= Date.parse(data.start) &&
          Date.parse(point.recordedAt) <= Date.parse(data.end),
      ) &&
      (data.baseline === null || Date.parse(data.baseline.recordedAt) < Date.parse(data.start)),
    'Chart observations must belong to their time window.',
  )

export const chartHistorySchema: z.ZodType<ChartHistory> = chartHistoryObject
export type ChartContractSchema = typeof chartHistoryObject
