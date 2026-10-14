import { afterEach, describe, expect, it, vi } from 'vitest'

// openapi-fetch binds fetch when the client module loads, so each test stubs fetch first and
// then imports a fresh module. Node requires absolute request URLs; the browser resolves them.
async function clientWith(response: Response) {
  vi.resetModules()
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(response)),
  )
  vi.stubGlobal(
    'Request',
    class extends Request {
      constructor(input: RequestInfo | URL, init?: RequestInit) {
        super(typeof input === 'string' ? new URL(input, 'https://console.example') : input, init)
      }
    },
  )
  return import('./client')
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('problem responses', () => {
  it('shows the problem detail and keeps the code', async () => {
    const { unlockSetup, APIError } = await clientWith(
      new Response(
        JSON.stringify({
          type: 'about:blank',
          title: 'Unauthorized',
          status: 401,
          detail: 'The setup token was not accepted.',
          code: 'invalid_token',
        }),
        { status: 401, headers: { 'Content-Type': 'application/problem+json' } },
      ),
    )
    const failure = await unlockSetup('token').catch((error: unknown) => error)
    expect(failure).toBeInstanceOf(APIError)
    expect(failure).toMatchObject({
      status: 401,
      code: 'invalid_token',
      message: 'The setup token was not accepted.',
    })
  })

  it('falls back to a generic message when the body is not problem details', async () => {
    const { unlockSetup } = await clientWith(
      new Response(JSON.stringify({ code: 'unknown', message: 'raw upstream text' }), {
        status: 502,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    await expect(unlockSetup('token')).rejects.toMatchObject({
      status: 502,
      code: null,
      message: 'Setup could not be unlocked.',
    })
  })
})
