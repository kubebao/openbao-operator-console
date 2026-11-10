import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import createClient from 'openapi-fetch'
import type { paths } from './generated'
import type { ProblemCode } from './problem'
import { problemSchema } from './problem'
import {
  historySchema,
  activityPageSchema,
  snapshotSchema,
  settingsSchema,
  connectionCheckSchema,
  setupStatusSchema,
  readerAccessSchema,
  loginRedirectSchema,
  credentialStatusesSchema,
} from './schema'
import {
  authenticatorEnrollmentSchema,
  localEnrollmentResultSchema,
  localSignInResultSchema,
  signInOptionsSchema,
} from './sign-in'
import type {
  AccessRules,
  Configuration,
  CredentialUpdate,
  ConnectionCheck,
  IdentityInput,
  ManagedConnection,
  SetupStatus,
} from './schema'
import type { LocalAdministratorInput } from './sign-in'

// client is shared with the sibling API modules.
export const client = createClient<paths>()
const firstPage: string | null = null

// Query keys are shared by queries and the mutations that refresh them.
export const queryKeys = {
  snapshot: ['snapshot'],
  setup: ['setup'],
  signIn: ['sign-in'],
  settings: ['settings'],
  credentialStatuses: ['credentialStatuses'],
  history: (id: string, since: string) => ['history', id, since] as const,
  activity: ['activity'],
} as const

export class APIError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    // The problem code, or null when the response carried no valid problem details.
    public readonly code: ProblemCode | null = null,
  ) {
    super(message)
    this.name = 'APIError'
  }
}

// readFailure reports a failed read with a fixed message for its view. The problem code is
// kept so callers can branch on it.
function readFailure(response: Response, body: unknown, message: string): APIError {
  const problem = problemSchema.safeParse(body)
  return new APIError(response.status, message, problem.success ? problem.data.code : null)
}

// requestFailure shows the server's problem detail, which is written for the requester.
// A body that is not valid problem details falls back to a generic message.
export function requestFailure(response: Response, body: unknown, fallback: string): APIError {
  const problem = problemSchema.safeParse(body)
  return problem.success
    ? new APIError(response.status, problem.data.detail, problem.data.code)
    : new APIError(response.status, fallback)
}

export const snapshotOptions = {
  queryKey: queryKeys.snapshot,
  queryFn: async () => {
    const response = await client.GET('/api/v1/snapshot')
    if (!response.response.ok)
      throw readFailure(response.response, response.error, 'Fleet evidence is unavailable.')
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

const setupOptions = {
  queryKey: queryKeys.setup,
  queryFn: async () => {
    const response = await client.GET('/api/v1/setup')
    if (!response.response.ok)
      throw readFailure(response.response, response.error, 'The console could not be reached.')
    return setupStatusSchema.parse(response.data)
  },
  // Retry transient failures. Client errors do not recover by retrying.
  retry: (count: number, error: Error) =>
    !(error instanceof APIError && error.status >= 400 && error.status < 500) && count < 2,
  staleTime: 15_000,
  refetchOnWindowFocus: true,
}

export function useSetup() {
  return useQuery(setupOptions)
}
export async function unlockSetup(token: string) {
  const response = await client.POST('/api/v1/setup/unlock', { body: { token } })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Setup could not be unlocked.')
  return setupStatusSchema.parse(response.data)
}
export async function startSetupIdentity(identity: IdentityInput) {
  const response = await client.POST('/api/v1/setup/identity', { body: identity })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Identity could not be verified.')
  return loginRedirectSchema.parse(response.data).url
}
export async function startSetupAuthenticator() {
  const response = await client.POST('/api/v1/setup/local/authenticator')
  if (!response.response.ok)
    throw requestFailure(
      response.response,
      response.error,
      'The authenticator could not be prepared.',
    )
  return authenticatorEnrollmentSchema.parse(response.data)
}
export async function createSetupAdministrator(input: LocalAdministratorInput) {
  const response = await client.POST('/api/v1/setup/local', { body: input })
  if (!response.response.ok)
    throw requestFailure(
      response.response,
      response.error,
      'The administrator could not be created.',
    )
  return localEnrollmentResultSchema.parse(response.data).recoveryCodes
}
export function useSignInOptions() {
  return useQuery({
    queryKey: queryKeys.signIn,
    queryFn: async () => {
      const response = await client.GET('/api/v1/sign-in')
      if (!response.response.ok)
        throw readFailure(response.response, response.error, 'Sign-in options could not be loaded.')
      return signInOptionsSchema.parse(response.data)
    },
    staleTime: 60_000,
  })
}
export async function signInLocal(username: string, password: string, code: string) {
  const response = await client.POST('/api/v1/sign-in/local', {
    body: { username, password, code },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Sign-in failed.')
  return localSignInResultSchema.parse(response.data)
}
export async function lockSetup() {
  const response = await client.POST('/api/v1/setup/lock')
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Setup could not be locked.')
  return setupStatusSchema.parse(response.data)
}

export function useHistory(id: string, since: string) {
  return useInfiniteQuery({
    queryKey: queryKeys.history(id, since),
    enabled: since !== '',
    initialPageParam: firstPage,
    queryFn: async ({ pageParam }) => {
      const response = await client.GET('/api/v1/deployments/{id}/history', {
        params: {
          path: { id },
          query: { since, limit: 100, ...(pageParam ? { cursor: pageParam } : {}) },
        },
      })
      if (!response.response.ok)
        throw readFailure(response.response, response.error, 'Observation history is unavailable.')
      return historySchema.parse(response.data)
    },
    getNextPageParam: (last) => last.nextCursor,
    select: (data) => ({
      observations: data.pages.flatMap((page) => page.observations),
      baseline: data.pages[0]?.baseline ?? null,
      nextCursor: data.pages.at(-1)?.nextCursor ?? null,
    }),
    retry: (count: number, error: Error) =>
      !(error instanceof APIError && error.status >= 400 && error.status < 500) && count < 2,
    staleTime: 30_000,
  })
}

export function useActivity(enabled = true) {
  const { data: snapshot } = useSnapshot(false)
  const scope =
    snapshot?.connections
      .map((connection) => [connection.id, ...connection.namespaces].join('|'))
      .sort()
      .join(';') ?? ''
  return useInfiniteQuery({
    queryKey: [...queryKeys.activity, snapshot?.viewer.subject, scope],
    enabled: enabled && Boolean(snapshot),
    initialPageParam: firstPage,
    queryFn: async ({ pageParam }) => {
      const response = await client.GET('/api/v1/activity', {
        params: { query: { limit: 100, ...(pageParam ? { cursor: pageParam } : {}) } },
      })
      if (!response.response.ok)
        throw readFailure(response.response, response.error, 'Recorded activity is unavailable.')
      return activityPageSchema.parse(response.data)
    },
    getNextPageParam: (last) => last.nextCursor,
    select: (data) => data.pages.flatMap((page) => page.events),
    retry: (count: number, error: Error) =>
      !(error instanceof APIError && error.status >= 400 && error.status < 500) && count < 2,
    staleTime: 15_000,
    refetchInterval: 30_000,
  })
}

export function useSettings(enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.settings,
    enabled,
    // Keep the edit revision stable. A concurrent administrator's save is
    // reported as a conflict instead of silently replacing the form's base.
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    queryFn: async () => {
      const response = await client.GET('/api/v1/settings')
      if (!response.response.ok)
        throw requestFailure(response.response, response.error, 'Settings unavailable.')
      return settingsSchema.parse(response.data)
    },
    retry: false,
  })
}
async function saveSettings(
  revision: number,
  configuration: Configuration,
  credentials: readonly CredentialUpdate[] = [],
) {
  const response = await client.PUT('/api/v1/settings', {
    body: { revision, configuration, credentials },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Settings were not saved.')
  return settingsSchema.parse(response.data)
}
export async function checkConnection(input: ConnectionCheck) {
  const response = await client.POST('/api/v1/settings/connections/check', { body: input })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Connection could not be checked.')
  return connectionCheckSchema.parse(response.data)
}
export async function generateReaderAccess(connection: ManagedConnection) {
  const response = await client.POST('/api/v1/settings/connections/rbac', {
    body: { connection },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Reader access could not be prepared.')
  return readerAccessSchema.parse(response.data)
}
// trialIdentity starts a candidate provider login. An issuer change requires an access
// rules decision; an unchanged issuer must omit it.
export async function trialIdentity(
  revision: number,
  identity: IdentityInput,
  accessRules: AccessRules | null,
) {
  const response = await client.POST('/api/v1/settings/identity/trial', {
    body: { revision, identity, ...(accessRules ? { accessRules } : {}) },
  })
  if (!response.response.ok)
    throw requestFailure(response.response, response.error, 'Identity could not be checked.')
  return loginRedirectSchema.parse(response.data).url
}

export function useCredentialStatuses(enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.credentialStatuses,
    enabled,
    queryFn: async () => {
      const response = await client.GET('/api/v1/settings/connections/credentials')
      if (!response.response.ok)
        throw readFailure(response.response, response.error, 'Credential status is unavailable.')
      return credentialStatusesSchema.parse(response.data)
    },
    refetchInterval: 15_000,
    retry: false,
  })
}

interface SettingsSave {
  revision: number
  configuration: Configuration
  credentials?: readonly CredentialUpdate[]
}

// useSaveSettings stores the new revision and refreshes every view derived from settings.
export function useSaveSettings() {
  const queries = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: ({ revision, configuration, credentials }: SettingsSave) =>
      saveSettings(revision, configuration, credentials),
    onSuccess: async (result) => {
      queries.setQueryData(queryKeys.settings, result)
      await Promise.all([
        queries.invalidateQueries({ queryKey: queryKeys.snapshot }),
        queries.resetQueries({ queryKey: queryKeys.activity }),
        queries.resetQueries({ queryKey: ['history'] }),
        queries.invalidateQueries({ queryKey: queryKeys.credentialStatuses }),
      ])
    },
  })
}

export function useCheckConnection() {
  // gcTime 0 drops cached variables, which can carry tokens, keys, and kubeconfigs, as soon
  // as no component observes the mutation. The same applies to every credential mutation.
  return useMutation({ mutationFn: (input: ConnectionCheck) => checkConnection(input), gcTime: 0 })
}

export function useReaderAccess() {
  return useMutation({
    mutationFn: (connection: ManagedConnection) => generateReaderAccess(connection),
  })
}

// useSetupStep runs a setup request and stores the returned status.
export function useSetupStep<T>(request: (input: T) => Promise<SetupStatus>) {
  const queries = useQueryClient()
  return useMutation({
    gcTime: 0,
    mutationFn: (input: T) => request(input),
    onSuccess: (status) => {
      queries.setQueryData(queryKeys.setup, status)
    },
  })
}
