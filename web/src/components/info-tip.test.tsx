import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { InfoTip } from './info-tip'

function tip() {
  const button = screen.getByRole('button', { name: 'About evidence' })
  const wrapper = button.parentElement
  if (!wrapper) throw new Error('info tip wrapper missing')
  return { button, wrapper }
}

describe('InfoTip', () => {
  it('describes its button with the tooltip text', () => {
    render(<InfoTip label="About evidence">Evidence can be stale.</InfoTip>)
    expect(tip().button).toHaveAccessibleDescription('Evidence can be stale.')
  })
  it('closes a hover-opened tooltip with Escape while focus is elsewhere', () => {
    render(<InfoTip label="About evidence">Evidence can be stale.</InfoTip>)
    const { wrapper } = tip()
    fireEvent.mouseEnter(wrapper)
    fireEvent.keyDown(document.body, { key: 'Escape' })
    expect(wrapper).toHaveClass('info-tip-dismissed')
    fireEvent.mouseLeave(wrapper)
    fireEvent.mouseEnter(wrapper)
    expect(wrapper).not.toHaveClass('info-tip-dismissed')
  })
  it('closes a focus-opened tooltip with Escape', () => {
    render(<InfoTip label="About evidence">Evidence can be stale.</InfoTip>)
    const { button, wrapper } = tip()
    fireEvent.focus(button)
    fireEvent.keyDown(button, { key: 'Escape' })
    expect(wrapper).toHaveClass('info-tip-dismissed')
  })
  it('ignores Escape when the tooltip is not shown', () => {
    render(<InfoTip label="About evidence">Evidence can be stale.</InfoTip>)
    fireEvent.keyDown(document.body, { key: 'Escape' })
    expect(tip().wrapper).not.toHaveClass('info-tip-dismissed')
  })
})
