import { useId, useState } from 'react'
import type { AccessRules, Configuration } from '../../api/schema'
import { affectedBindings } from '../../lib/identity'
import { FormError } from '../form'
import { SettingsEditor } from '../settings-editor'
import { Button } from '../ui/button'

const choices: readonly { value: AccessRules; title: string; detail: string }[] = [
  {
    value: 'remove',
    title: 'Remove existing access',
    detail:
      'Only the account you sign in with keeps access, as an administrator. Add other users and groups again for the new issuer.',
  },
  {
    value: 'carryOver',
    title: 'Carry over to the new issuer',
    detail:
      'Every listed user and group keeps its access under the new issuer. Choose this only if the new provider issues the same subjects and group names for the same people.',
  },
]

// AccessRulesDialog asks what happens to user and group access before an issuer change.
// Bindings belong to one issuer, so none of them moves to another issuer silently.
export function AccessRulesDialog({
  activeIssuer,
  candidateIssuer,
  configuration,
  pending,
  error,
  onConfirm,
  onClose,
}: {
  activeIssuer: string
  candidateIssuer: string
  configuration: Configuration
  pending: boolean
  error: unknown
  onConfirm: (accessRules: AccessRules) => void
  onClose: () => void
}) {
  const [choice, setChoice] = useState<AccessRules | null>(null)
  const listHeading = useId()
  const bindings = affectedBindings(configuration)
  return (
    <SettingsEditor
      title="Choose access for the new issuer"
      description={`User and group access belongs to ${activeIssuer}. The candidate provider uses ${candidateIssuer}. Choose what happens to this access before you sign in.`}
      onClose={onClose}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault()
          if (choice) onConfirm(choice)
        }}
      >
        <h3 id={listHeading} className="access-rules-heading">
          Affected users and groups ({bindings.length})
        </h3>
        <ul className="affected-access" aria-labelledby={listHeading}>
          {bindings.map((binding) => (
            <li key={`${binding.kind}:${binding.key}`}>
              <span>{binding.label}</span>
              <small>{binding.detail}</small>
            </li>
          ))}
        </ul>
        <fieldset className="access-rules-choice">
          <legend>Existing access</legend>
          {choices.map((option) => (
            <label key={option.value}>
              <input
                type="radio"
                name="access-rules"
                value={option.value}
                required
                checked={choice === option.value}
                onChange={() => {
                  setChoice(option.value)
                }}
              />
              <span>
                <strong>{option.title}</strong>
                <small>{option.detail}</small>
              </span>
            </label>
          ))}
        </fieldset>
        <p className="muted">
          The account you sign in with becomes an administrator under the new issuer. Activation
          ends existing console sessions.
        </p>
        <FormError error={error} />
        <div className="editor-actions">
          <Button type="button" variant="ghost" onClick={onClose} disabled={pending}>
            Cancel
          </Button>
          <Button type="submit" variant="default" disabled={pending}>
            {pending ? 'Checking…' : 'Verify login and activate'}
          </Button>
        </div>
      </form>
    </SettingsEditor>
  )
}
