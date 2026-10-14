import { useState } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import {
  createMemoryHistory,
  createRootRoute,
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SetupStatus, Snapshot } from '../api/schema'
import { APIError } from '../api/client'
import { snapshotFixture } from '../test/fixtures'
import { Shell } from './shell'

// The shell reads setup and fleet evidence through these hooks. The tests replace their
// requests and keep TanStack Query's real result states.
const requests = vi.hoisted(() => ({
  setup: vi.fn<() => Promise<SetupStatus>>(),
  snapshot: vi.fn<() => Promise<Snapshot>>(),
}))

vi.mock('../api/client', async (importOriginal) => {
  const actual = await importOriginal<object>()
  return {
    ...actual,
    useSetup: () => useQuery({ queryKey: ['setup'], queryFn: requests.setup, retry: false }),
    useSnapshot: () =>
      useQuery({ queryKey: ['snapshot'], queryFn: requests.snapshot, retry: false }),
  }
})

const complete: SetupStatus = {
  state: 'complete',
  callbackUrl: 'https://console.example/auth/callback',
  sessionExpiresAt: null,
}

function DraftPage() {
  const [draft, setDraft] = useState('')
  return (
    <label>
      Reader kubeconfig
      <textarea
        value={draft}
        onChange={(event) => {
          setDraft(event.target.value)
        }}
      />
    </label>
  )
}

function renderShell() {
  const queryClient = new QueryClient()
  const root = createRootRoute({ component: Shell })
  const page = createRoute({ getParentRoute: () => root, path: '/', component: DraftPage })
  const router = createRouter({
    routeTree: root.addChildren([page]),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  )
  return queryClient
}

async function typeDraft() {
  const draft = await screen.findByRole('textbox', { name: 'Reader kubeconfig' })
  fireEvent.change(draft, { target: { value: 'fixture-kubeconfig' } })
  return draft
}

describe('Shell', () => {
  beforeEach(() => {
    requests.setup.mockResolvedValue(complete)
    requests.snapshot.mockResolvedValue(snapshotFixture())
  })

  it('blocks the workspace when the first evidence load fails', async () => {
    requests.snapshot.mockRejectedValue(new APIError(503, 'Fleet evidence is unavailable.'))
    renderShell()
    expect(
      await screen.findByRole('heading', { name: 'Fleet evidence unavailable' }),
    ).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: 'Reader kubeconfig' })).toBeNull()
  })

  it('keeps the page and typed input when a background refresh fails', async () => {
    const queryClient = renderShell()
    const draft = await typeDraft()
    requests.snapshot.mockRejectedValue(new APIError(503, 'Fleet evidence is unavailable.'))
    await act(() => queryClient.refetchQueries({ queryKey: ['snapshot'] }))
    expect(await screen.findByText('The console could not refresh.')).toBeInTheDocument()
    expect(draft).toHaveValue('fixture-kubeconfig')
    expect(screen.queryByRole('heading', { name: 'Fleet evidence unavailable' })).toBeNull()

    requests.snapshot.mockResolvedValue(snapshotFixture())
    fireEvent.click(screen.getByRole('button', { name: 'Retry now' }))
    await waitFor(() => {
      expect(screen.queryByText('The console could not refresh.')).toBeNull()
    })
    expect(draft).toHaveValue('fixture-kubeconfig')
  })

  it('keeps the workspace when a setup status refresh fails', async () => {
    const queryClient = renderShell()
    const draft = await typeDraft()
    requests.setup.mockRejectedValue(new APIError(502, 'The console could not be reached.'))
    await act(() => queryClient.refetchQueries({ queryKey: ['setup'] }))
    expect(await screen.findByText('The console could not refresh.')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Console unavailable' })).toBeNull()
    expect(draft).toHaveValue('fixture-kubeconfig')
  })

  it('asks the viewer to sign in again when evidence returns 401 after load', async () => {
    const queryClient = renderShell()
    await typeDraft()
    requests.snapshot.mockRejectedValue(new APIError(401, 'Sign in.'))
    await act(() => queryClient.refetchQueries({ queryKey: ['snapshot'] }))
    expect(
      await screen.findByRole('link', { name: 'Sign in with single sign-on' }),
    ).toBeInTheDocument()
  })

  it('names the signed-in viewer instead of a placeholder', async () => {
    renderShell()
    await typeDraft()
    expect(screen.getByText('Fixture Viewer')).toBeInTheDocument()
    expect(screen.queryByText('Platform team')).toBeNull()
  })
})
