import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { newConnection } from '../../lib/connection'
import { connectionCheck, emptyCredential } from '../../lib/credential'
import { managedConnectionSchema } from '../../api/schema'
import { ClusterStep } from './cluster-step'

describe('operator API selection', () => {
  it('defaults new enrollment to the native API and carries selection into checks', () => {
    const connection = newConnection()
    expect(connection.apiGroup).toBe('kubebao.org')
    const onChange = vi.fn()
    render(
      <ClusterStep connection={connection} onName={vi.fn()} onID={vi.fn()} onChange={onChange} />,
    )
    const selector = screen.getByRole('combobox', { name: /^Operator API group/ })
    expect(selector).toHaveValue('kubebao.org')
    fireEvent.change(selector, { target: { value: 'openbao.org' } })
    expect(onChange).toHaveBeenCalledWith({ ...connection, apiGroup: 'openbao.org' })
    const legacy = { ...connection, apiGroup: 'openbao.org' as const }
    expect(connectionCheck(legacy, emptyCredential('token')).connection.apiGroup).toBe(
      'openbao.org',
    )
    expect(managedConnectionSchema.parse(legacy).apiGroup).toBe('openbao.org')
    expect(managedConnectionSchema.safeParse({ ...legacy, apiGroup: 'other.org' }).success).toBe(
      false,
    )
  })
})
