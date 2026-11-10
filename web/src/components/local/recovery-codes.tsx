import { useState } from 'react'
import type { ReactNode } from 'react'
import { Download } from 'lucide-react'
import { CodeBlock } from '../code-block'
import { Button } from '../ui/button'
import { downloadText } from '../../lib/download'

// RecoveryCodes shows single-use codes once. The action stays disabled until the owner
// confirms that the codes are stored.
export function RecoveryCodes({
  codes,
  onError,
  children,
}: {
  codes: readonly string[]
  onError: (message: string) => void
  children: (confirmed: boolean) => ReactNode
}) {
  const [confirmed, setConfirmed] = useState(false)
  const text = codes.join('\n')
  return (
    <section className="recovery-codes" aria-labelledby="recovery-codes-heading">
      <h2 id="recovery-codes-heading">Save your recovery codes</h2>
      <p>
        Each code signs you in once when your authenticator is unavailable. They are shown only now.
        Store them in a password manager or another safe place.
      </p>
      <CodeBlock code={text} label="Recovery codes" onCopyError={onError} />
      <div className="form-actions">
        <Button
          type="button"
          onClick={() => {
            downloadText(`${text}\n`, 'openbao-console-recovery-codes.txt', 'text/plain')
          }}
        >
          <Download size={14} aria-hidden="true" />
          Download codes
        </Button>
      </div>
      <label className="checkbox-field">
        <input
          type="checkbox"
          checked={confirmed}
          onChange={(event) => {
            setConfirmed(event.target.checked)
          }}
        />
        I stored these recovery codes in a safe place
      </label>
      {children(confirmed)}
    </section>
  )
}
