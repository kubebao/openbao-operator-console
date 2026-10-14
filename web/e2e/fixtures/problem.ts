import type { ProblemCode } from '../../src/api/problem'

const titles: Partial<Record<number, string>> = {
  400: 'Bad Request',
  401: 'Unauthorized',
  409: 'Conflict',
  503: 'Service Unavailable',
}

// problem builds an RFC 9457 response as the server writes it.
export function problem(status: number, code: ProblemCode, detail: string) {
  return {
    status,
    contentType: 'application/problem+json',
    body: JSON.stringify({
      type: 'about:blank',
      title: titles[status] ?? 'Error',
      status,
      detail,
      code,
    }),
  }
}
