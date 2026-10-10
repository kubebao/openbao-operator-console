import { describe, expect, it } from 'vitest'
import { addNamespace, newConnection, updateNamespace } from './connection'
import { emptyCredential } from './credential'
import { clusterID, scopeKey, stepProblem, withOperatorScope } from './onboarding'

const named = { ...newConnection(), name: 'Production EU', id: 'production-eu-1234' }

describe('clusterID', () => {
  it('derives a slug with a suffix', () => {
    expect(clusterID('Production EU', 'abcd')).toBe('production-eu-abcd')
    expect(clusterID('***', 'abcd')).toBe('cluster-abcd')
  })
})

describe('stepProblem', () => {
  const draft = emptyCredential('kubeconfig')

  it('requires a name and an unused identifier', () => {
    expect(stepProblem('Cluster', named, draft, [])).toBeNull()
    expect(stepProblem('Cluster', { ...named, id: 'bad id' }, draft, [])).toMatch(
      /valid identifier/,
    )
    expect(stepProblem('Cluster', named, draft, [named.id])).toMatch(/already enrolled/)
  })

  it('requires valid, unique namespaces', () => {
    const scoped = updateNamespace(named, 0, { name: 'team-a' })
    expect(stepProblem('Scope', scoped, draft, [])).toBeNull()
    const duplicate = updateNamespace(addNamespace(scoped), 1, { name: 'team-a' })
    expect(stepProblem('Scope', duplicate, draft, [])).toMatch(/unique/)
    expect(stepProblem('Scope', updateNamespace(scoped, 0, { name: 'Team' }), draft, [])).toMatch(
      /valid/,
    )
  })

  it('requires a complete remote credential', () => {
    expect(stepProblem('Credentials', named, draft, [])).toMatch(/complete reader credential/)
    expect(stepProblem('Credentials', { ...named, authentication: 'local' }, draft, [])).toBeNull()
  })
})

describe('withOperatorScope', () => {
  it('assigns every named namespace to the operator', () => {
    const connection = {
      ...updateNamespace(addNamespace(updateNamespace(named, 0, { name: 'team-a' })), 1, {
        name: 'team-b',
      }),
      operators: [
        {
          id: 'operator',
          namespace: 'system',
          deployment: 'controller',
          container: 'manager',
          namespaces: [],
        },
      ],
    }
    expect(withOperatorScope(connection).operators[0]?.namespaces).toEqual(['team-a', 'team-b'])
  })
})

describe('scopeKey', () => {
  it('ignores the API server', () => {
    expect(scopeKey({ ...named, server: 'https://a.example' })).toBe(scopeKey(named))
  })
})
