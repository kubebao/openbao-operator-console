import * as Dialog from '@radix-ui/react-dialog'
import { X } from 'lucide-react'
import { useRef } from 'react'
import type { ReactNode } from 'react'
import { Button } from './ui/button'

// SettingsEditor is a modal settings dialog. A description, when given, is visible and
// announced with the title; otherwise a generic description is announced only.
export function SettingsEditor({
  title,
  description,
  onClose,
  children,
}: {
  title: string
  description?: string | undefined
  onClose: () => void
  children: ReactNode
}) {
  // Editors mount already open, without a Radix trigger. Retain their opener so
  // Escape, Cancel, and successful saves return keyboard focus to the same control.
  const opener = useRef(document.activeElement)
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content
          className="settings-dialog"
          onCloseAutoFocus={(event) => {
            if (opener.current instanceof HTMLElement && opener.current.isConnected) {
              event.preventDefault()
              opener.current.focus()
            }
          }}
        >
          <div className="section-heading">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close asChild>
              <Button size="icon" variant="ghost" aria-label="Close editor">
                <X size={18} />
              </Button>
            </Dialog.Close>
          </div>
          {description ? (
            <Dialog.Description className="dialog-description">{description}</Dialog.Description>
          ) : (
            <Dialog.Description className="sr-only">
              Manage console configuration.
            </Dialog.Description>
          )}
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}
