import { fireEvent, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { Configuration, CredentialUpdate } from '../../api/schema'
import { settingsFixture } from '../../test/fixtures'
import { renderWithQueries } from '../../test/render'
import { ConnectionDialog } from './connection-dialog'

const scoped = settingsFixture({
  users: [
    {
      subject: 'engineer',
      name: 'Engineer',
      admin: false,
      scopes: [{ connectionId: 'fixture-cluster', namespaces: ['team-a', 'team-b'] }],
    },
  ],
}).configuration

function setup(configuration: Configuration) {
  const commit = vi.fn<
    (next: Configuration, credentials?: readonly CredentialUpdate[]) => Promise<void>
  >(() => Promise.resolve())
  const onClose = vi.fn()
  const connection = configuration.connections[0]
  if (!connection) throw new Error('missing fixture connection')
  renderWithQueries(
    <ConnectionDialog
      connection={connection}
      configuration={configuration}
      pending={false}
      commit={commit}
      onClose={onClose}
    />,
  )
  return { commit, onClose }
}

describe('ConnectionDialog', () => {
  it('confirms access removed with a namespace before saving', async () => {
    const { commit, onClose } = setup(scoped)
    fireEvent.click(screen.getByRole('button', { name: 'Remove namespace 2' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    const confirm = await screen.findByRole('dialog', { name: 'Remove access and relationships?' })
    expect(within(confirm).getByRole('list', { name: 'Access removed' })).toHaveTextContent(
      'Engineer (user) loses fixture-cluster/team-b',
    )
    expect(commit).not.toHaveBeenCalled()

    // Keeping the edit returns to the open editor with the change intact.
    fireEvent.click(within(confirm).getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => {
      expect(screen.queryByRole('dialog', { name: 'Remove access and relationships?' })).toBeNull()
    })
    expect(screen.queryByRole('button', { name: 'Remove namespace 2' })).toBeNull()
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    const again = await screen.findByRole('dialog', { name: 'Remove access and relationships?' })
    fireEvent.click(within(again).getByRole('button', { name: 'Save and remove' }))
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    const saved = commit.mock.calls[0]?.[0]
    expect(saved?.users[0]?.scopes).toEqual([
      { connectionId: 'fixture-cluster', namespaces: ['team-a'] },
    ])
    expect(saved?.connections[0]?.namespaces.map((namespace) => namespace.name)).toEqual(['team-a'])
  })
  it('saves without confirmation when nothing references the change', async () => {
    const { commit, onClose } = setup(scoped)
    fireEvent.change(screen.getByRole('textbox', { name: 'Region' }), {
      target: { value: 'eu-west' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save connection' }))
    await waitFor(() => {
      expect(onClose).toHaveBeenCalled()
    })
    expect(screen.queryByRole('dialog', { name: 'Remove access and relationships?' })).toBeNull()
    expect(commit.mock.calls[0]?.[0].connections[0]?.region).toBe('eu-west')
  })
})
