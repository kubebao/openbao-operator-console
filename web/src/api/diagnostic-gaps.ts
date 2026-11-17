import { z } from 'zod'

export const diagnosticGap = z.object({
  rule: z.string(),
  reasons: z
    .array(
      z.enum([
        'excluded',
        'not_requested',
        'scope_incomplete',
        'missing_observation',
        'stale_evidence',
        'missing_field',
        'generation_unconfirmed',
        'inconsistent_time',
        'not_configured',
        'higher_priority_backup_signal',
        'target_unverified',
        'association_unconfirmed',
        'scope_restricted',
        'unknown',
      ]),
    )
    .min(1)
    .max(14)
    .refine((values) => new Set(values).size === values.length),
  collectionOutcome: z.enum([
    'not_requested',
    'not_collected',
    'complete',
    'partial',
    'denied',
    'expired',
    'rate_limited',
    'unsupported',
    'timeout',
    'cancelled',
    'page_limit',
    'size_limit',
    'pagination_loop',
    'deleted',
    'replaced',
    'invalid',
    'failed',
    'unknown',
  ]),
  checkedAt: z.iso.datetime({ offset: true }),
})
