import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import type { Configuration, Settings } from '../../api/schema'
import { Field, FormError } from '../form'
import { Button } from '../ui/button'

export function PolicySettings({
  settings,
  onSave,
}: {
  settings: Settings
  onSave: (configuration: Configuration) => Promise<unknown>
}) {
  const [hours, setHours] = useState(settings.configuration.maxBackupAgeHours)
  const [grace, setGrace] = useState(settings.configuration.startupGraceMinutes)
  const save = useMutation({ mutationFn: (configuration: Configuration) => onSave(configuration) })
  return (
    <form
      className="settings-form"
      onSubmit={(event) => {
        event.preventDefault()
        save.mutate({
          ...settings.configuration,
          maxBackupAgeHours: hours,
          startupGraceMinutes: grace,
        })
      }}
    >
      <h2>Attention policies</h2>
      <Field
        label="Maximum backup age (hours)"
        help="A configured backup older than this threshold needs attention."
      >
        <input
          type="number"
          required
          min={1}
          value={hours}
          onChange={(event) => {
            setHours(event.target.valueAsNumber)
          }}
        />
      </Field>
      <Field
        label="Startup grace (minutes)"
        help="Delay availability findings while a new deployment starts."
      >
        <input
          type="number"
          required
          min={0}
          value={grace}
          onChange={(event) => {
            setGrace(event.target.valueAsNumber)
          }}
        />
      </Field>
      <FormError error={save.error} />
      <Button type="submit" variant="default" disabled={save.isPending}>
        {save.isPending ? 'Saving…' : 'Save policies'}
      </Button>
    </form>
  )
}
