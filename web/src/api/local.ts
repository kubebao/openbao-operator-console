import { z } from 'zod'
import type { components } from './generated'

// Runtime schemas for local account sign-in and setup.

export type SignInOptions = components['schemas']['SignInOptions']
export type LocalSignInResult = components['schemas']['LocalSignInResult']
export type AuthenticatorEnrollment = components['schemas']['AuthenticatorEnrollment']
export type LocalAdministratorInput = components['schemas']['LocalAdministratorInput']

const signInOptionsObject = z.object({ singleSignOn: z.boolean(), localAccounts: z.boolean() })
const localSignInResultObject = z.object({
  recoveryCodeUsed: z.boolean(),
  recoveryCodesLeft: z.number().int().min(0).max(10),
})
const authenticatorEnrollmentObject = z.object({
  key: z.string().regex(/^[A-Z2-7]+$/),
  uri: z.string().startsWith('otpauth://totp/'),
  qrSize: z.number().int().min(21).max(200),
  // Path data contains only move, relative line, and close commands with integers.
  qrPath: z.string().regex(/^[Mhvz0-9 -]*$/),
})
const localEnrollmentResultObject = z.object({ recoveryCodes: z.array(z.string()).length(10) })

export const signInOptionsSchema: z.ZodType<SignInOptions> = signInOptionsObject
export const localSignInResultSchema: z.ZodType<LocalSignInResult> = localSignInResultObject
export const authenticatorEnrollmentSchema: z.ZodType<AuthenticatorEnrollment> =
  authenticatorEnrollmentObject
export const localEnrollmentResultSchema: z.ZodType<
  components['schemas']['LocalEnrollmentResult']
> = localEnrollmentResultObject

// LocalContractSchemas exposes the unannotated schemas to the contract test.
const localContractSchemas = {
  signInOptions: signInOptionsObject,
  localSignInResult: localSignInResultObject,
  authenticatorEnrollment: authenticatorEnrollmentObject,
  localEnrollmentResult: localEnrollmentResultObject,
}
export type LocalContractSchemas = typeof localContractSchemas
