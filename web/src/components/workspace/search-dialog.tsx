import { useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useNavigate } from '@tanstack/react-router'
import * as Dialog from '@radix-ui/react-dialog'
import { Boxes, ChevronRight, Search, X } from 'lucide-react'
import type { Deployment } from '../../api/schema'
import { HealthBadge } from '../status'
import { Button } from '../ui/button'

const resultLimit = 8

function matches(deployment: Deployment, query: string): boolean {
  return `${deployment.name} ${deployment.namespace} ${deployment.team} ${deployment.connectionId}`
    .toLocaleLowerCase()
    .includes(query.toLocaleLowerCase())
}

function SearchResult({
  deployment,
  active,
  onActivate,
  onOpen,
}: {
  deployment: Deployment
  active: boolean
  onActivate: () => void
  onOpen: () => void
}) {
  return (
    <button
      id={`search-result-${deployment.id}`}
      role="option"
      aria-selected={active}
      tabIndex={-1}
      className={active ? 'active' : ''}
      onMouseMove={onActivate}
      onClick={onOpen}
    >
      <Boxes size={18} />
      <span>
        <strong>{deployment.name}</strong>
        <small>
          {deployment.connectionId} / {deployment.namespace}
        </small>
      </span>
      <HealthBadge health={deployment.health} />
      <ChevronRight size={16} />
    </button>
  )
}

// SearchDialog finds deployments in the authorized snapshot and opens the selected one.
export function SearchDialog({
  deployments,
  open,
  onOpenChange,
}: {
  deployments: readonly Deployment[]
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [active, setActive] = useState(0)
  const results = deployments
    .filter((deployment) => matches(deployment, query))
    .slice(0, resultLimit)
  const selected = Math.min(active, Math.max(results.length - 1, 0))
  const current = results[selected]
  const openDeployment = (deploymentId: string) => {
    onOpenChange(false)
    void navigate({ to: '/deployments/$deploymentId', params: { deploymentId } })
  }
  const onKeyDown = (event: KeyboardEvent<HTMLInputElement>) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setActive(Math.min(selected + 1, results.length - 1))
    } else if (event.key === 'ArrowUp') {
      event.preventDefault()
      setActive(Math.max(selected - 1, 0))
    } else if (event.key === 'Enter' && current) {
      event.preventDefault()
      openDeployment(current.id)
    }
  }
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="search-dialog">
          <Dialog.Title className="sr-only">Search fleet</Dialog.Title>
          <Dialog.Description className="sr-only">
            Find deployments by name, team, or Kubernetes cluster.
          </Dialog.Description>
          <div className="command-input">
            <Search size={18} />
            <input
              autoFocus
              placeholder="Search your fleet…"
              value={query}
              role="combobox"
              aria-expanded={results.length > 0}
              aria-controls="search-results"
              aria-activedescendant={current ? `search-result-${current.id}` : undefined}
              onChange={(event) => {
                setQuery(event.target.value)
                setActive(0)
              }}
              onKeyDown={onKeyDown}
              aria-label="Search your fleet"
            />
            <Dialog.Close asChild>
              <Button size="icon" variant="ghost" aria-label="Close search">
                <X size={17} />
              </Button>
            </Dialog.Close>
          </div>
          <div className="command-results">
            <div className="nav-label" id="search-results-label">
              DEPLOYMENTS
            </div>
            {results.length === 0 && <p className="muted">No deployments match this search.</p>}
            <div role="listbox" id="search-results" aria-labelledby="search-results-label">
              {results.map((deployment, index) => (
                <SearchResult
                  key={deployment.id}
                  deployment={deployment}
                  active={index === selected}
                  onActivate={() => {
                    setActive(index)
                  }}
                  onOpen={() => {
                    openDeployment(deployment.id)
                  }}
                />
              ))}
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
