import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import type { Configuration, Settings } from '../../api/schema'
import { wholeNumber } from '../../lib/settings'
import { Field, FormError } from '../form'
import { Button } from '../ui/button'

// PolicySettings edits the attention thresholds. Inputs keep their typed text, so an empty
// or partial value stays visible and is rejected on save instead of becoming NaN.
export function PolicySettings({
  settings,
  onSave,
}: {
  settings: Settings
  onSave: (configuration: Configuration) => Promise<unknown>
}) {
  const [hours, setHours] = useState(String(settings.configuration.maxBackupAgeHours))
  const [grace, setGrace] = useState(String(settings.configuration.startupGraceMinutes))
  const [invalid, setInvalid] = useState('')
  const save = useMutation({ mutationFn: (configuration: Configuration) => onSave(configuration) })
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        const maxBackupAgeHours = wholeNumber(hours, 1)
        const startupGraceMinutes = wholeNumber(grace, 0)
        if (maxBackupAgeHours === null || startupGraceMinutes === null) {
          setInvalid(
            'Enter a whole number of at least 1 hour for backup age and at least 0 minutes for startup grace.',
          )
          return
        }
        setInvalid('')
        save.mutate({ ...settings.configuration, maxBackupAgeHours, startupGraceMinutes })
      }}
    >
      <h2>Attention policies</h2>
      <Field
        label="Maximum backup age (hours)"
        help="A configured backup older than this threshold needs attention."
      >
        <input
          type="number"
          inputMode="numeric"
          required
          min={1}
          step={1}
          value={hours}
          onChange={(event) => {
            setHours(event.target.value)
          }}
        />
      </Field>
      <Field
        label="Startup grace (minutes)"
        help="Delay availability findings while a new deployment starts."
      >
        <input
          type="number"
          inputMode="numeric"
          required
          min={0}
          step={1}
          value={grace}
          onChange={(event) => {
            setGrace(event.target.value)
          }}
        />
      </Field>
      <FormError error={invalid || save.error} />
      <Button type="submit" variant="default" disabled={save.isPending}>
        {save.isPending ? 'Saving…' : 'Save policies'}
      </Button>
    </form>
  )
}
