import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from './ui/button'
import type { Configuration, Scope } from '../api/schema'

export function SettingsEditor({
  title,
  onClose,
  children,
}: {
  title: string
  onClose: () => void
  children: ReactNode
}) {
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="settings-dialog">
          <div className="section-heading">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close asChild>
              <Button size="icon" variant="ghost" aria-label="Close editor">
                <X size={18} />
              </Button>
            </Dialog.Close>
          </div>
          <Dialog.Description className="sr-only">Manage console configuration.</Dialog.Description>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
export function Field({
  label,
  children,
  help,
}: {
  label: string
  children: ReactNode
  help?: string
}) {
  return (
    <label className="settings-field">
      <span>{label}</span>
      {children}
      {help && <small>{help}</small>}
    </label>
  )
}
export function ScopePicker({
  configuration,
  scopes,
  onChange,
}: {
  configuration: Configuration
  scopes: readonly Scope[]
  onChange: (scopes: readonly Scope[]) => void
}) {
  return (
    <fieldset className="scope-picker">
      <legend>Visible namespaces</legend>
      {configuration.connections.length === 0 && <p>Add a cluster before assigning namespaces.</p>}
      {configuration.connections.map((connection) => (
        <div key={connection.id}>
          <strong>{connection.name}</strong>
          {connection.namespaces.map((namespace) => {
            const checked = scopes.some(
              (scope) =>
                scope.connectionId === connection.id && scope.namespaces.includes(namespace.name),
            )
            return (
              <label key={namespace.name}>
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={(event) => {
                    const remaining = scopes.filter((scope) => scope.connectionId !== connection.id)
                    const names = scopes
                      .filter((scope) => scope.connectionId === connection.id)
                      .flatMap((scope) => scope.namespaces)
                      .filter((name) => name !== namespace.name)
                    if (event.target.checked) names.push(namespace.name)
                    onChange(
                      names.length
                        ? [...remaining, { connectionId: connection.id, namespaces: names }]
                        : remaining,
                    )
                  }}
                />
                {namespace.name}
              </label>
            )
          })}
        </div>
      ))}
    </fieldset>
  )
}
