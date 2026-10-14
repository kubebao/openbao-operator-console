import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { deploymentFixture } from '../../test/fixtures'
import { EvidenceTiles } from './evidence-tiles'

function renderTiles(version: string, desiredVersion: string) {
  render(
    <EvidenceTiles
      deployment={deploymentFixture({ version, desiredVersion })}
      generatedAt="2026-10-06T12:00:00Z"
      maxBackupAgeHours={24}
      onSelect={() => undefined}
    />,
  )
  return screen.getByRole('button', { name: 'Lifecycle: open Lifecycle' })
}

describe('lifecycle tile', () => {
  it('reports an unknown upgrade state when the version is not observed', () => {
    const tile = renderTiles('', '2.7.0')
    expect(tile).toHaveTextContent('Version not observed')
    expect(tile).toHaveTextContent('Upgrade state unknown')
    expect(tile).not.toHaveTextContent('settled')
  })
  it('reports a settled upgrade when both versions match', () => {
    expect(renderTiles('2.7.0', '2.7.0')).toHaveTextContent('Upgrade settled')
  })
  it('shows the requested version during an upgrade', () => {
    const tile = renderTiles('2.7.0', '2.8.0')
    expect(tile).toHaveTextContent('2.7.0 → 2.8.0')
    expect(tile).toHaveTextContent('Upgrade in progress')
  })
})
