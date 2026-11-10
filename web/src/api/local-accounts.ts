import { useMutation, useQueryClient } from '@tanstack/react-query'
import { client, queryKeys, requestFailure } from './client'
import { settingsSchema } from './schema'
import type { Scope, Settings } from './schema'
import {
  enrollmentStartSchema,
  localAccountEnrollmentSchema,
  localEnrollmentResultSchema,
} from './sign-in'
import type { LocalAccountEnrollment } from './sign-in'

// Local account administration, enrollment, and self-service requests.

export interface LocalAccountFields {
  name: string
  admin: boolean
  scopes: readonly Scope[]
}

async function createLocalAccount(revision: number, username: string, fields: LocalAccountFields) {
  const response = await client.POST('/api/v1/settings/local-accounts', {
    body: { revision, username, ...fields },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'The account could not be created.')
  return localAccountEnrollmentSchema.parse(response.data)
}

async function updateLocalAccount(revision: number, id: string, fields: LocalAccountFields) {
  const response = await client.PUT('/api/v1/settings/local-accounts/{id}', {
    params: { path: { id } },
    body: { revision, ...fields },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'The account could not be saved.')
  return settingsSchema.parse(response.data)
}

async function deleteLocalAccount(revision: number, id: string) {
  const response = await client.DELETE('/api/v1/settings/local-accounts/{id}', {
    params: { path: { id }, query: { revision } },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'The account could not be removed.')
  return settingsSchema.parse(response.data)
}

async function resetLocalAccount(id: string) {
  const response = await client.POST('/api/v1/settings/local-accounts/{id}/enrollment', {
    params: { path: { id } },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'The account could not be reset.')
  return localAccountEnrollmentSchema.parse(response.data)
}

async function setLocalSignIn(revision: number, enabled: boolean) {
  const response = await client.PUT('/api/v1/settings/local-sign-in', {
    body: { revision, enabled },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Local sign-in could not be changed.')
  return settingsSchema.parse(response.data)
}

export type LocalAccountAction =
  | { kind: 'create'; revision: number; username: string; fields: LocalAccountFields }
  | { kind: 'update'; revision: number; id: string; fields: LocalAccountFields }
  | { kind: 'delete'; revision: number; id: string }
  | { kind: 'reset'; id: string }
  | { kind: 'signIn'; revision: number; enabled: boolean }

// LocalAccountResult is the updated settings, or a one-time enrollment link.
export type LocalAccountResult =
  { kind: 'settings'; settings: Settings } | { kind: 'link'; enrollment: LocalAccountEnrollment }

async function runLocalAccountAction(action: LocalAccountAction): Promise<LocalAccountResult> {
  switch (action.kind) {
    case 'create':
      return {
        kind: 'link',
        enrollment: await createLocalAccount(action.revision, action.username, action.fields),
      }
    case 'update':
      return {
        kind: 'settings',
        settings: await updateLocalAccount(action.revision, action.id, action.fields),
      }
    case 'delete':
      return { kind: 'settings', settings: await deleteLocalAccount(action.revision, action.id) }
    case 'reset':
      return { kind: 'link', enrollment: await resetLocalAccount(action.id) }
    case 'signIn':
      return { kind: 'settings', settings: await setLocalSignIn(action.revision, action.enabled) }
  }
}

// useLocalAccountAction runs an administration request and refreshes settings. Requests
// that return settings replace the cached value; enrollment links trigger a reload.
export function useLocalAccountAction() {
  const queries = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: runLocalAccountAction,
    onSuccess: async (result) => {
      if (result.kind === 'settings') queries.setQueryData(queryKeys.settings, result.settings)
      else await queries.invalidateQueries({ queryKey: queryKeys.settings })
    },
  })
}

export async function openEnrollment(token: string) {
  const response = await client.POST('/api/v1/enrollment', { body: { token } })
  if (!response.response.ok)
    throw requestFailure(
      response.response,
      response.error,
      'The enrollment link could not be opened.',
    )
  return enrollmentStartSchema.parse(response.data)
}

export async function completeEnrollment(token: string, password: string, code: string) {
  const response = await client.POST('/api/v1/enrollment/complete', {
    body: { token, password, code },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Enrollment could not be completed.')
  return localEnrollmentResultSchema.parse(response.data).recoveryCodes
}

export async function changeAccountPassword(
  currentPassword: string,
  newPassword: string,
  code: string,
) {
  const response = await client.POST('/api/v1/account/password', {
    body: { currentPassword, newPassword, code },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'The password could not be changed.')
}

export async function replaceRecoveryCodes(password: string, code: string) {
  const response = await client.POST('/api/v1/account/recovery-codes', { body: { password, code } })
  if (!response.response.ok)
    throw requestFailure(
      response.response,
      response.error,
      'The recovery codes could not be replaced.',
    )
  return localEnrollmentResultSchema.parse(response.data).recoveryCodes
}
