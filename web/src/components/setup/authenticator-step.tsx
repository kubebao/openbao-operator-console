import { Smartphone } from 'lucide-react'
import type { AuthenticatorEnrollment } from '../../api/sign-in'
import { Field } from '../form'
import { QRCode } from '../local/qr-code'
import { secretFieldProps } from '../../lib/credential'
import { Button } from '../ui/button'

// AuthenticatorStep shows the TOTP secret as a QR code and a manual key, and asks for
// the current code to prove the app works.
export function AuthenticatorStep({
  enrollment,
  preparing,
  disabled,
  code,
  onCode,
  onPrepare,
}: {
  enrollment: AuthenticatorEnrollment | undefined
  preparing: boolean
  disabled: boolean
  code: string
  onCode: (code: string) => void
  onPrepare: () => void
}) {
  if (!enrollment)
    return (
      <div className="form-actions">
        <Button type="button" disabled={disabled} onClick={onPrepare}>
          <Smartphone size={16} aria-hidden="true" />
          {preparing ? 'Preparing…' : 'Add an authenticator app'}
        </Button>
      </div>
    )
  return (
    <div className="authenticator-enrollment">
      <QRCode
        size={enrollment.qrSize}
        path={enrollment.qrPath}
        label="QR code for your authenticator app"
      />
      <div>
        <p>Scan the QR code with an authenticator app, or enter this key manually:</p>
        <code className="authenticator-key">{enrollment.key}</code>
        <Field label="Authenticator code" help="The six-digit code your app shows now.">
          <input
            {...secretFieldProps}
            value={code}
            onChange={(event) => {
              onCode(event.target.value)
            }}
            inputMode="numeric"
            pattern="[0-9 ]{6,7}"
            maxLength={7}
            autoComplete="one-time-code"
            required
          />
        </Field>
      </div>
    </div>
  )
}
