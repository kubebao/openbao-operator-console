import { describe, expect, it } from 'vitest'
import { newConnection } from './connection'
import {
  checkKey,
  connectionCheck,
  credentialComplete,
  credentialUpdate,
  emptyCredential,
  withRenewal,
} from './credential'

const remote = { ...newConnection(), id: 'cluster', server: 'https://api.example' }

describe('connectionCheck', () => {
  it('sends no credential for the hosting cluster', () => {
    const local = { ...remote, authentication: 'local' as const }
    expect(connectionCheck(local, { ...emptyCredential('token'), token: 'ignored' })).toEqual({
      connection: local,
    })
  })

  it('omits an empty kubeconfig context', () => {
    const draft = { ...emptyCredential('kubeconfig'), kubeconfig: 'apiVersion: v1' }
    expect(connectionCheck(remote, draft)).toEqual({
      connection: remote,
      kubeconfig: 'apiVersion: v1',
    })
    expect(connectionCheck(remote, { ...draft, context: 'reader' }).context).toBe('reader')
  })

  it('sends only the fields of the selected method', () => {
    const draft = {
      ...emptyCredential('token'),
      token: 'secret',
      certificate: 'stale',
      key: 'stale',
    }
    expect(connectionCheck(remote, draft).credential).toEqual({
      server: 'https://api.example',
      ca: '',
      token: 'secret',
      certificate: '',
      key: '',
    })
  })

  it('keeps a stored credential when nothing changed', () => {
    const stored = { server: 'https://api.example' }
    expect(connectionCheck(remote, emptyCredential('token'), stored)).toEqual({
      connection: remote,
    })
    const moved = { ...remote, server: 'https://moved.example' }
    expect(connectionCheck(moved, emptyCredential('token'), stored).credential?.server).toBe(
      'https://moved.example',
    )
  })
})

describe('credentialUpdate', () => {
  it('maps a check onto the credential to save', () => {
    const token = connectionCheck(remote, { ...emptyCredential('token'), token: 'secret' })
    expect(credentialUpdate(token)?.credential?.token).toBe('secret')
    const kubeconfig = connectionCheck(remote, {
      ...emptyCredential('kubeconfig'),
      kubeconfig: 'config',
    })
    expect(credentialUpdate(kubeconfig)).toEqual({ connectionId: 'cluster', kubeconfig: 'config' })
    expect(credentialUpdate({ connection: remote })).toBeUndefined()
  })
})

describe('credentialComplete', () => {
  it('requires every field of the selected method', () => {
    expect(credentialComplete(remote, emptyCredential('kubeconfig'))).toBe(false)
    expect(credentialComplete(remote, { ...emptyCredential('token'), token: 'secret' })).toBe(true)
    expect(
      credentialComplete(remote, { ...emptyCredential('certificate'), certificate: 'pem' }),
    ).toBe(false)
    expect(
      credentialComplete(
        { ...remote, server: '' },
        { ...emptyCredential('token'), token: 'secret' },
      ),
    ).toBe(false)
  })
})

describe('withRenewal', () => {
  it('replaces a client certificate with a token method', () => {
    const draft = { ...emptyCredential('certificate'), certificate: 'pem', key: 'key' }
    expect(withRenewal(draft, true)).toMatchObject({ method: 'token', certificate: '', key: '' })
    expect(withRenewal(draft, false)).toBe(draft)
  })
})

describe('checkKey', () => {
  it('treats a paused connection like an enabled one', () => {
    expect(checkKey({ connection: { ...remote, disabled: true } })).toBe(
      checkKey({ connection: remote }),
    )
  })
})
