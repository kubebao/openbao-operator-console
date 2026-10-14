import { fireEvent, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { emptyFilter } from '../../lib/filters'
import { deploymentFixture, snapshotFixture } from '../../test/fixtures'
import { renderInApp } from '../../test/render'
import { IssueTable } from '../attention/issue-table'
import { QuickViews } from './quick-views'

describe('segmented controls', () => {
  it('expose the selected quick view as a pressed button in a named group', async () => {
    renderInApp(
      <QuickViews
        deployments={[deploymentFixture()]}
        filter={{ ...emptyFilter, health: 'critical' }}
        onView={() => undefined}
      />,
    )
    const group = await screen.findByRole('group', { name: 'Quick views' })
    expect(group).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^Critical/ })).toHaveAttribute(
      'aria-pressed',
      'true',
    )
    expect(screen.getByRole('button', { name: /^All deployments/ })).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })
  it('move the pressed state with the selected issue category', async () => {
    renderInApp(<IssueTable snapshot={snapshotFixture()} />)
    await screen.findByRole('group', { name: 'Issue category' })
    const all = screen.getByRole('button', { name: /^All issues/ })
    const backup = screen.getByRole('button', { name: /^Backup protection/ })
    expect(all).toHaveAttribute('aria-pressed', 'true')
    fireEvent.click(backup)
    expect(backup).toHaveAttribute('aria-pressed', 'true')
    expect(all).toHaveAttribute('aria-pressed', 'false')
  })
})
