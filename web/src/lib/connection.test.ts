import { describe, expect, it } from 'vitest'
import {
  addNamespace,
  assignNamespace,
  newConnection,
  removeNamespace,
  updateNamespace,
  withAuthentication,
} from './connection'

function connectionWithOperator() {
  const connection = updateNamespace(newConnection(), 0, { name: 'team-a' })
  return {
    ...addNamespace(connection),
    operators: [
      {
        id: 'operator',
        namespace: 'system',
        deployment: 'controller',
        container: 'manager',
        namespaces: ['team-a'],
      },
    ],
  }
}

describe('namespace edits', () => {
  it('keeps a renamed namespace assigned to its operator', () => {
    const renamed = updateNamespace(connectionWithOperator(), 0, { name: 'team-x' })
    expect(renamed.namespaces[0]?.name).toBe('team-x')
    expect(renamed.operators[0]?.namespaces).toEqual(['team-x'])
  })

  it('does not assign a new namespace to an operator', () => {
    const named = updateNamespace(connectionWithOperator(), 1, { name: 'team-b' })
    expect(named.operators[0]?.namespaces).toEqual(['team-a'])
  })

  it('removes a namespace from every operator', () => {
    const removed = removeNamespace(connectionWithOperator(), 0)
    expect(removed.namespaces).toHaveLength(1)
    expect(removed.operators[0]?.namespaces).toEqual([])
  })

  it('adds new namespaces without claiming an environment', () => {
    expect(addNamespace(newConnection()).namespaces[1]?.environment).toBe('other')
  })

  it('assigns and unassigns namespaces once', () => {
    const connection = updateNamespace(connectionWithOperator(), 1, { name: 'team-b' })
    const assigned = assignNamespace(
      assignNamespace(connection, 0, 'team-b', true),
      0,
      'team-b',
      true,
    )
    expect(assigned.operators[0]?.namespaces).toEqual(['team-a', 'team-b'])
    expect(assignNamespace(assigned, 0, 'team-a', false).operators[0]?.namespaces).toEqual([
      'team-b',
    ])
  })
})

describe('authentication', () => {
  it('drops renewal for the hosting cluster', () => {
    const remote = {
      ...newConnection(),
      renewal: { namespace: 'console-system', serviceAccount: 'reader' },
    }
    expect(withAuthentication(remote, 'local').renewal).toBeNull()
    expect(withAuthentication(remote, 'remote').renewal).toEqual(remote.renewal)
  })
})
