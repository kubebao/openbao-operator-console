import { useState } from 'react'
import { LockKeyhole } from 'lucide-react'
import { Field } from '../form'
import { Button } from '../ui/button'

// UnlockForm clears the token as soon as it is submitted.
export function UnlockForm({
  pending,
  onUnlock,
}: {
  pending: boolean
  onUnlock: (token: string) => void
}) {
  const [token, setToken] = useState('')
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        onUnlock(token)
        setToken('')
      }}
    >
      <p>Enter the private setup token supplied with this installation.</p>
      <Field label="Setup token">
        <input
          type="password"
          value={token}
          onChange={(event) => {
            setToken(event.target.value)
          }}
          autoComplete="off"
          spellCheck={false}
          required
          maxLength={256}
          autoFocus
        />
      </Field>
      <div className="form-actions">
        <Button type="submit" variant="default" disabled={pending}>
          <LockKeyhole size={16} aria-hidden="true" />
          {pending ? 'Opening setup…' : 'Unlock setup'}
        </Button>
      </div>
    </form>
  )
}
