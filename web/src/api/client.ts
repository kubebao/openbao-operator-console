import { useQuery } from '@tanstack/react-query'
import createClient from 'openapi-fetch'
import type { paths } from './generated'
import {
  historySchema,
  snapshotSchema,
  settingsSchema,
  connectionCheckSchema,
  setupStatusSchema,
  readerAccessSchema,
  loginRedirectSchema,
  credentialStatusesSchema,
} from './schema'
import type {
  Configuration,
  CredentialUpdate,
  ConnectionCheck,
  IdentityInput,
  ManagedConnection,
} from './schema'

const client = createClient<paths>()

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message)
    this.name = 'APIError'
  }
}

export const snapshotOptions = {
  queryKey: ['snapshot'],
  queryFn: async () => {
    const response = await client.GET('/api/v1/snapshot')
    if (!response.response.ok)
      throw new APIError(response.response.status, 'Fleet evidence is unavailable.')
    return snapshotSchema.parse(response.data)
  },
  refetchInterval: 30_000,
  staleTime: 15_000,
  retry: (count: number, error: Error) =>
    !(error instanceof APIError && error.status === 401) && count < 2,
}

export function useSnapshot(enabled = true) {
  return useQuery({ ...snapshotOptions, enabled })
}

export const setupOptions = {
  queryKey: ['setup'],
  queryFn: async () => {
    const response = await client.GET('/api/v1/setup')
    if (!response.response.ok)
      throw new APIError(response.response.status, 'The console could not be reached.')
    return setupStatusSchema.parse(response.data)
  },
  retry: false,
  staleTime: 15_000,
  refetchOnWindowFocus: true,
}

export function useSetup() {
  return useQuery(setupOptions)
}
export async function unlockSetup(token: string) {
  const response = await client.POST('/api/v1/setup/unlock', { body: { token } })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Setup could not be unlocked.',
    )
  return setupStatusSchema.parse(response.data)
}
export async function startSetupIdentity(identity: IdentityInput) {
  const response = await client.POST('/api/v1/setup/identity', { body: identity })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Identity could not be verified.',
    )
  return loginRedirectSchema.parse(response.data).url
}
export async function lockSetup() {
  const response = await client.POST('/api/v1/setup/lock')
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Setup could not be locked.',
    )
  return setupStatusSchema.parse(response.data)
}

export function useHistory(id: string) {
  return useQuery({
    queryKey: ['history', id],
    queryFn: async () => {
      const response = await client.GET('/api/v1/deployments/{id}/history', {
        params: { path: { id } },
      })
      if (!response.response.ok)
        throw new APIError(response.response.status, 'Observation history is unavailable.')
      return historySchema.parse(response.data)
    },
    staleTime: 30_000,
  })
}

export function useSettings(enabled: boolean) {
  return useQuery({
    queryKey: ['settings'],
    enabled,
    // Keep the edit revision stable. A concurrent administrator's save is
    // reported as a conflict instead of silently replacing the form's base.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    queryFn: async () => {
      const response = await client.GET('/api/v1/settings')
      if (!response.response.ok)
        throw new APIError(
          response.response.status,
          response.error?.message ?? 'Settings unavailable.',
        )
      return settingsSchema.parse(response.data)
    },
    retry: false,
  })
}
export async function saveSettings(
  revision: number,
  configuration: Configuration,
  credentials: readonly CredentialUpdate[] = [],
) {
  const response = await client.PUT('/api/v1/settings', {
    body: { revision, configuration, credentials },
  })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Settings were not saved.',
    )
  return settingsSchema.parse(response.data)
}
export async function checkConnection(input: ConnectionCheck) {
  const response = await client.POST('/api/v1/settings/connections/check', { body: input })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Connection could not be checked.',
    )
  return connectionCheckSchema.parse(response.data)
}
export async function generateReaderAccess(connection: ManagedConnection) {
  const response = await client.POST('/api/v1/settings/connections/rbac', {
    body: { connection },
  })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Reader access could not be prepared.',
    )
  return readerAccessSchema.parse(response.data)
}
export async function trialIdentity(revision: number, identity: IdentityInput) {
  const response = await client.POST('/api/v1/settings/identity/trial', {
    body: { revision, identity },
  })
  if (!response.response.ok)
    throw new APIError(
      response.response.status,
      response.error?.message ?? 'Identity could not be checked.',
    )
  return loginRedirectSchema.parse(response.data).url
}

export function useCredentialStatuses(enabled: boolean) {
  return useQuery({
    queryKey: ['credentialStatuses'],
    enabled,
    queryFn: async () => {
      const response = await client.GET('/api/v1/settings/connections/credentials')
      if (!response.response.ok)
        throw new APIError(response.response.status, 'Credential status is unavailable.')
      return credentialStatusesSchema.parse(response.data)
    },
    refetchInterval: 15_000,
    retry: false,
  })
}
