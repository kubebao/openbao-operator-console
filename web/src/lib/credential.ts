import type { ConnectionCheck, CredentialUpdate, ManagedConnection } from '../api/schema'

export type CredentialMethod = 'kubeconfig' | 'token' | 'certificate'

export function isCredentialMethod(value: string): value is CredentialMethod {
  return value === 'kubeconfig' || value === 'token' || value === 'certificate'
}

// CredentialDraft holds reader credential input before it is checked or saved. The API
// server is part of the connection.
export interface CredentialDraft {
  method: CredentialMethod
  kubeconfig: string
  context: string
  ca: string
  token: string
  certificate: string
  key: string
}

export function emptyCredential(method: CredentialMethod): CredentialDraft {
  return { method, kubeconfig: '', context: '', ca: '', token: '', certificate: '', key: '' }
}

// withRenewal keeps the draft compatible with token renewal, which requires a bearer token.
export function withRenewal(draft: CredentialDraft, renewal: boolean): CredentialDraft {
  if (!renewal || draft.method !== 'certificate') return draft
  return { ...draft, method: 'token', certificate: '', key: '' }
}

function entered(draft: CredentialDraft): boolean {
  if (draft.method === 'kubeconfig') return draft.kubeconfig !== ''
  return draft.ca !== '' || draft.token !== '' || draft.certificate !== '' || draft.key !== ''
}

// connectionCheck builds the request that checks or saves a connection. When the
// connection already has a stored credential, an empty draft with an unchanged API server
// keeps it.
export function connectionCheck(
  connection: ManagedConnection,
  draft: CredentialDraft,
  stored?: { server: string },
): ConnectionCheck {
  if (connection.authentication === 'local') return { connection }
  if (stored && !entered(draft) && connection.server === stored.server) return { connection }
  if (draft.method === 'kubeconfig')
    return {
      connection,
      kubeconfig: draft.kubeconfig,
      ...(draft.context ? { context: draft.context } : {}),
    }
  return {
    connection,
    credential: {
      server: connection.server,
      ca: draft.ca,
      token: draft.method === 'token' ? draft.token : '',
      certificate: draft.method === 'certificate' ? draft.certificate : '',
      key: draft.method === 'certificate' ? draft.key : '',
    },
  }
}

export function credentialUpdate(check: ConnectionCheck): CredentialUpdate | undefined {
  const connectionId = check.connection.id
  if (check.credential) return { connectionId, credential: check.credential }
  if (check.kubeconfig !== undefined)
    return {
      connectionId,
      kubeconfig: check.kubeconfig,
      ...(check.context ? { context: check.context } : {}),
    }
  return undefined
}

// credentialComplete reports whether a new remote connection has a complete credential.
export function credentialComplete(connection: ManagedConnection, draft: CredentialDraft): boolean {
  if (connection.authentication === 'local') return true
  switch (draft.method) {
    case 'kubeconfig':
      return draft.kubeconfig.trim() !== ''
    case 'token':
      return connection.server !== '' && draft.token !== ''
    case 'certificate':
      return connection.server !== '' && draft.certificate !== '' && draft.key !== ''
  }
}

// checkKey identifies the input a check result belongs to. A paused connection is checked
// like an enabled one.
export function checkKey(check: ConnectionCheck): string {
  return JSON.stringify({ ...check, connection: { ...check.connection, disabled: false } })
}
