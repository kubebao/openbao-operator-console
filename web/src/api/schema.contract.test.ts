import { describe, expectTypeOf, it } from 'vitest'
import type { z } from 'zod'
import type { components } from './generated'
import type { ActivityContractSchema } from './activity'
import type { ProblemContractSchema } from './problem'
import type { ContractSchemas } from './schema'
import type { LocalContractSchemas } from './local'

// Type-level contract checks, enforced by `pnpm typecheck`. The z.ZodType<T> annotations in
// schema.ts only prove that parsed values fit the generated types. These checks prove the
// reverse: every generated value, including every enum member, fits its schema. A contract
// change that adds an enum member therefore fails typecheck instead of failing at runtime.

type Contract = components['schemas']

// Generated types are immutable; schema inputs are mutable. Compare them as read-only.
type DeepReadonly<T> = T extends readonly (infer U)[]
  ? readonly DeepReadonly<U>[]
  : T extends object
    ? { readonly [K in keyof T]: DeepReadonly<T[K]> }
    : T

type Input<K extends keyof ContractSchemas> = DeepReadonly<z.input<ContractSchemas[K]>>
type LocalInput<K extends keyof LocalContractSchemas> = DeepReadonly<
  z.input<LocalContractSchemas[K]>
>

describe('runtime schemas accept the generated contract', () => {
  it('accepts every fleet value', () => {
    expectTypeOf<Contract['Snapshot']>().toExtend<Input<'snapshot'>>()
    expectTypeOf<Contract['History']>().toExtend<Input<'history'>>()
    expectTypeOf<Contract['ActivityPage']>().toExtend<Input<'activityPage'>>()
    expectTypeOf<Contract['Activity']>().toExtend<DeepReadonly<z.input<ActivityContractSchema>>>()
    expectTypeOf<Contract['ConnectionCheckResult']>().toExtend<Input<'connectionCheck'>>()
  })
  it('accepts every administration value', () => {
    expectTypeOf<Contract['Settings']>().toExtend<Input<'settings'>>()
    expectTypeOf<Contract['SettingsUpdate']>().toExtend<Input<'settingsUpdate'>>()
    expectTypeOf<Contract['IdentityTrial']>().toExtend<Input<'identityTrial'>>()
    expectTypeOf<Contract['CredentialStatuses']>().toExtend<Input<'credentialStatuses'>>()
    expectTypeOf<Contract['ReaderAccessResult']>().toExtend<Input<'readerAccess'>>()
  })
  it('accepts every setup and login value', () => {
    expectTypeOf<Contract['SetupStatus']>().toExtend<Input<'setupStatus'>>()
    expectTypeOf<Contract['LoginRedirect']>().toExtend<Input<'loginRedirect'>>()
    expectTypeOf<Contract['SignInOptions']>().toExtend<LocalInput<'signInOptions'>>()
    expectTypeOf<Contract['LocalSignInResult']>().toExtend<LocalInput<'localSignInResult'>>()
    expectTypeOf<Contract['AuthenticatorEnrollment']>().toExtend<
      LocalInput<'authenticatorEnrollment'>
    >()
    expectTypeOf<Contract['LocalEnrollmentResult']>().toExtend<
      LocalInput<'localEnrollmentResult'>
    >()
  })
  it('accepts every problem value', () => {
    expectTypeOf<Contract['Problem']>().toExtend<DeepReadonly<z.input<ProblemContractSchema>>>()
  })
})
