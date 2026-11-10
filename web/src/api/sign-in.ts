import { z } from 'zod'
import type { components } from './generated'

// Runtime schemas for setup, sign-in, enrollment, and local accounts.

export type SignInOptions = components['schemas']['SignInOptions']
export type LocalSignInResult = components['schemas']['LocalSignInResult']
export type AuthenticatorEnrollment = components['schemas']['AuthenticatorEnrollment']
export type LocalAdministratorInput = components['schemas']['LocalAdministratorInput']

export type LocalAccount = components['schemas']['LocalAccount']

// localAccountObject is part of the settings schema.
export const localAccountObject = z.object({
  id: z.string(),
  username: z.string(),
  name: z.string(),
  admin: z.boolean(),
  scopes: z.array(z.object({ connectionId: z.string(), namespaces: z.array(z.string()) })),
  state: z.enum(['active', 'enrollmentPending', 'enrollmentExpired']),
  enrollmentExpiresAt: z.iso.datetime({ offset: true }).nullable(),
})

export type SetupStatus = components['schemas']['SetupStatus']

const setupStatusObject = z.object({
  state: z.enum(['required', 'unlocked', 'complete', 'expired']),
  callbackUrl: z.string(),
  sessionExpiresAt: z.iso.datetime({ offset: true }).nullable(),
})
const loginRedirectObject = z.object({
  url: z.url().refine((value) => value.startsWith('https://'), 'Login URL must use HTTPS.'),
})

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

export const setupStatusSchema: z.ZodType<SetupStatus> = setupStatusObject
export const loginRedirectSchema: z.ZodType<components['schemas']['LoginRedirect']> =
  loginRedirectObject
export const signInOptionsSchema: z.ZodType<SignInOptions> = signInOptionsObject
export const localSignInResultSchema: z.ZodType<LocalSignInResult> = localSignInResultObject
export const authenticatorEnrollmentSchema: z.ZodType<AuthenticatorEnrollment> =
  authenticatorEnrollmentObject
export const localEnrollmentResultSchema: z.ZodType<
  components['schemas']['LocalEnrollmentResult']
> = localEnrollmentResultObject

// SignInContractSchemas exposes the unannotated schemas to the contract test.
const signInContractSchemas = {
  setupStatus: setupStatusObject,
  loginRedirect: loginRedirectObject,
  signInOptions: signInOptionsObject,
  localSignInResult: localSignInResultObject,
  authenticatorEnrollment: authenticatorEnrollmentObject,
  localEnrollmentResult: localEnrollmentResultObject,
}
export type SignInContractSchemas = typeof signInContractSchemas
