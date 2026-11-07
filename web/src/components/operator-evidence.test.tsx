import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { renderInApp } from '../test/render'
import { deploymentFixture, operatorFixture, snapshotFixture } from '../test/fixtures'
import { DeploymentHeader } from './deployment/header'
import { OperatorEvidence } from './operator-evidence'
import { OperatorHealthBadge } from './status'

describe('operator health', () => {
  it('shows health from current collection', () => {
    render(<OperatorHealthBadge operator={operatorFixture()} />)
    expect(screen.getByText('Available')).toHaveClass('status-healthy')
  })
  it('does not present the last reported health as current when collection is stale', () => {
    render(<OperatorHealthBadge operator={operatorFixture({ collection: 'stale' })} />)
    expect(screen.queryByText('Available')).not.toBeInTheDocument()
    expect(screen.getByText('Health unknown')).toHaveClass('status-unknown')
  })
  it('keeps the last reported health as a labelled detail', () => {
    render(<OperatorEvidence operator={operatorFixture({ collection: 'unreachable' })} />)
    expect(screen.getByText('Health unknown')).toBeInTheDocument()
    expect(screen.getByText('Unreachable')).toBeInTheDocument()
    expect(screen.getByText('Last reported health').nextElementSibling).toHaveTextContent(
      'Available',
    )
  })
  it('marks the deployment header operator as unknown with its collection state', async () => {
    const connection = snapshotFixture().connections[0]
    renderInApp(
      <DeploymentHeader
        deployment={deploymentFixture()}
        connection={connection}
        operator={operatorFixture({ collection: 'notFound' })}
      />,
    )
    expect(await screen.findByText('Health unknown')).toHaveClass('status-unknown')
    expect(screen.getByText('Deployment missing')).toBeInTheDocument()
    expect(screen.queryByText('Available')).not.toBeInTheDocument()
  })
  it('separates producer evidence time from collection time', async () => {
    renderInApp(
      <DeploymentHeader
        deployment={deploymentFixture()}
        connection={undefined}
        operator={undefined}
      />,
    )
    expect(await screen.findByText('Producer time not reported')).toHaveClass('status-unknown')
    expect(screen.queryByText('Freshness unknown')).not.toBeInTheDocument()
    expect(screen.getByText(/^Collected/)).toBeInTheDocument()
  })
})
