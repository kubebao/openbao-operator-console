import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Settings } from '../../api/schema'
import { IdentitySettings } from './identity-settings'

const trialIdentity = vi.hoisted(() => vi.fn(() => new Promise<string>(() => undefined)))
vi.mock('../../api/client', () => ({ trialIdentity }))

const settings: Settings = {
  revision: 3,
  configuration: {
    connections: [],
    users: [
      { subject: '42', name: 'Ada', admin: true, scopes: [] },
      { subject: '7', name: 'Reader', admin: false, scopes: [] },
    ],
    groups: [{ name: 'platform', admin: false, scopes: [] }],
    relationships: [],
    maxBackupAgeHours: 24,
    startupGraceMinutes: 10,
  },
  identity: {
    issuer: 'https://keycloak.example/realms/platform',
    clientId: 'console',
    ca: '',
    groupsClaim: 'groups',
    secretConfigured: true,
  },
  changes: [],
}

function renderSettings() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <IdentitySettings settings={settings} />
    </QueryClientProvider>,
  )
}

afterEach(() => {
  cleanup()
  trialIdentity.mockClear()
})

describe('identity settings', () => {
  it('keeps access without a decision when the issuer is unchanged', async () => {
    renderSettings()
    fireEvent.click(screen.getByRole('button', { name: 'Verify login and activate' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    await waitFor(() => {
      expect(trialIdentity).toHaveBeenCalledWith(
        3,
        expect.objectContaining({ issuer: settings.identity.issuer }),
        null,
      )
    })
  })

  it('lists affected access and requires a decision before an issuer change', async () => {
    renderSettings()
    fireEvent.change(screen.getByRole('textbox', { name: 'Issuer' }), {
      target: { value: 'https://gitlab.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Review access changes' }))
    const dialog = screen.getByRole('dialog', { name: 'Choose access for the new issuer' })
    expect(dialog).toHaveAccessibleDescription(/belongs to https:\/\/keycloak\.example/)
    const affected = within(dialog).getByRole('list', { name: 'Affected users and groups (3)' })
    expect(within(affected).getAllByRole('listitem')).toHaveLength(3)
    expect(within(affected).getByText('Administrator · subject 42')).toBeInTheDocument()
    // The modal moves focus into the dialog.
    expect(dialog.contains(document.activeElement)).toBe(true)

    fireEvent.click(within(dialog).getByRole('button', { name: 'Verify login and activate' }))
    expect(trialIdentity).not.toHaveBeenCalled()

    fireEvent.click(within(dialog).getByRole('radio', { name: /Remove existing access/ }))
    fireEvent.click(within(dialog).getByRole('button', { name: 'Verify login and activate' }))
    await waitFor(() => {
      expect(trialIdentity).toHaveBeenCalledWith(
        3,
        expect.objectContaining({ issuer: 'https://gitlab.example' }),
        'remove',
      )
    })
  })

  it('cancels an issuer change without starting a login', () => {
    renderSettings()
    fireEvent.change(screen.getByRole('textbox', { name: 'Issuer' }), {
      target: { value: 'https://gitlab.example' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Review access changes' }))
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trialIdentity).not.toHaveBeenCalled()
  })
})
