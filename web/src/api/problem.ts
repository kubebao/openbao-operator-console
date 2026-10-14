import { z } from 'zod'
import type { components } from './generated'

export type Problem = components['schemas']['Problem']
export type ProblemCode = components['schemas']['ProblemCode']

// RFC 9457 problem details as the console serves them.
const problemObject = z.object({
  type: z.literal('about:blank'),
  title: z.string(),
  status: z.number().int().min(400).max(599),
  detail: z.string(),
  code: z.enum([
    'invalid',
    'unauthenticated',
    'invalid_token',
    'setup_locked',
    'forbidden',
    'not_found',
    'conflict',
    'setup_complete',
    'setup_expired',
    'rate_limited',
    'unavailable',
    'setup_required',
  ]),
})

export const problemSchema: z.ZodType<Problem> = problemObject

// ProblemContractSchema exposes the unannotated schema to the contract test.
export type ProblemContractSchema = typeof problemObject
