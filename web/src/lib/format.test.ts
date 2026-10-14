import { describe, expect, it } from 'vitest'
import { age, initials, until } from './format'

describe('evidence time labels', () => {
  const now = Date.parse('2026-10-06T12:00:00Z')
  it('keeps missing and invalid timestamps unknown', () => {
    expect(age(null, now)).toBe('Not observed')
    expect(age('invalid', now)).toBe('Unknown')
  })
  it('does not describe a clock ahead as fresh', () => {
    expect(age('2026-10-06T13:00:00Z', now)).toBe('Clock ahead')
  })
  it('labels completion age without an RPO claim', () => {
    expect(age('2026-10-06T11:46:00Z', now)).toBe('14m ago')
  })
  it('labels long ages in years', () => {
    expect(age('2024-10-01T12:00:00Z', now)).toBe('2y ago')
  })
})

describe('scheduled time labels', () => {
  const now = Date.parse('2026-10-06T12:00:00Z')
  it('describes future and missed schedules', () => {
    expect(until('2026-10-06T15:00:00Z', now)).toBe('in 3h')
    expect(until('2026-10-06T09:00:00Z', now)).toBe('3h ago')
    expect(until(null, now)).toBe('Not scheduled')
  })
})

describe('viewer initials', () => {
  it('abbreviates the signed-in name and falls back to the subject', () => {
    expect(initials({ name: 'Ada Lovelace King', subject: 'ada' })).toBe('AL')
    expect(initials({ name: '  ', subject: 'service-account' })).toBe('S')
    expect(initials({ name: '', subject: '' })).toBe('?')
  })
})
