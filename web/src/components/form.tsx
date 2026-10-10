import type { ReactNode } from 'react'
import { formError } from '../lib/errors'

export function Field({
  label,
  children,
  help,
}: {
  label: string
  children: ReactNode
  help?: string | undefined
}) {
  return (
    <label className="settings-field">
      <span>{label}</span>
      {children}
      {help && <small>{help}</small>}
    </label>
  )
}

// FormError announces a failed action. Follow-up actions render after the message.
export function FormError({ error, children }: { error: unknown; children?: ReactNode }) {
  if (!error) return null
  const message = typeof error === 'string' ? error : formError(error)
  if (!children)
    return (
      <p className="form-error" role="alert">
        {message}
      </p>
    )
  return (
    <div className="form-error" role="alert">
      {message}
      {children}
    </div>
  )
}
