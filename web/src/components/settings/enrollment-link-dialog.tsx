import type { LocalAccountEnrollment } from '../../api/sign-in'
import { CodeBlock } from '../code-block'
import { SettingsEditor } from '../settings-editor'
import { Button } from '../ui/button'
import { absolute } from '../../lib/format'

// EnrollmentLinkDialog shows a one-time enrollment link once.
export function EnrollmentLinkDialog({
  username,
  enrollment,
  onError,
  onClose,
}: {
  username: string
  enrollment: LocalAccountEnrollment
  onError: (message: string) => void
  onClose: () => void
}) {
  return (
    <SettingsEditor
      title={`Enrollment link for ${username}`}
      description="Share this link privately with the account owner. It is shown only once."
      onClose={onClose}
    >
      <CodeBlock code={enrollment.enrollmentUrl} label="Enrollment link" onCopyError={onError} />
      <p className="muted">
        The owner opens the link, sets a password, and adds an authenticator app. The link works
        once and expires {absolute(enrollment.expiresAt)}. To replace it, reset the account.
      </p>
      <div className="editor-actions">
        <Button variant="default" onClick={onClose}>
          Done
        </Button>
      </div>
    </SettingsEditor>
  )
}
