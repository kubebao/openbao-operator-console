import { useState } from 'react'
import { Plus } from 'lucide-react'
import type { Settings } from '../../api/schema'
import type { LocalAccount, LocalAccountEnrollment } from '../../api/sign-in'
import { useLocalAccountAction } from '../../api/local-accounts'
import type { LocalAccountResult } from '../../api/local-accounts'
import { absolute, plural } from '../../lib/format'
import { scopedNamespaceCount } from '../../lib/settings'
import { FormError } from '../form'
import { SettingsEditor } from '../settings-editor'
import { Button } from '../ui/button'
import { EnrollmentLinkDialog } from './enrollment-link-dialog'
import { LocalAccountEditor } from './local-account-editor'

type Dialog =
  | { kind: 'add' }
  | { kind: 'edit'; account: LocalAccount }
  | { kind: 'reset'; account: LocalAccount }
  | { kind: 'remove'; account: LocalAccount }
  | { kind: 'link'; username: string; enrollment: LocalAccountEnrollment }

function accountState(account: LocalAccount): string {
  switch (account.state) {
    case 'active':
      return 'Active'
    case 'enrollmentPending':
      return `Enrollment link valid until ${absolute(account.enrollmentExpiresAt ?? '')}`
    case 'enrollmentExpired':
      return 'Enrollment link expired. Reset the account for a new link.'
  }
}

function localSignInHelp(enabled: boolean, provider: boolean): string {
  if (!enabled)
    return 'Local accounts cannot sign in, and their sessions ended. Turn local sign-in on, or use local administrator recovery, to sign in with them again.'
  if (provider)
    return 'Local accounts can sign in beside single sign-on. Turn local sign-in off to keep them only for recovery.'
  return 'Local accounts are the only sign-in. Add an identity provider before turning local sign-in off.'
}

function LocalSignInSetting({
  settings,
  pending,
  onChange,
}: {
  settings: Settings
  pending: boolean
  onChange: (enabled: boolean) => void
}) {
  const provider = settings.identity.issuer !== ''
  return (
    <div className="local-sign-in-setting">
      <div>
        <h3>Local sign-in {settings.localSignIn ? 'on' : 'off'}</h3>
        <p className="muted">{localSignInHelp(settings.localSignIn, provider)}</p>
      </div>
      <Button
        disabled={pending || (settings.localSignIn && !provider)}
        onClick={() => {
          onChange(!settings.localSignIn)
        }}
      >
        Turn local sign-in {settings.localSignIn ? 'off' : 'on'}
      </Button>
    </div>
  )
}

function AccountRow({
  account,
  onEdit,
  onReset,
  onRemove,
}: {
  account: LocalAccount
  onEdit: () => void
  onReset: () => void
  onRemove: () => void
}) {
  return (
    <article className="settings-row">
      <div>
        <h3>{account.name || account.username}</h3>
        <p>
          {account.username} · {accountState(account)}
        </p>
      </div>
      <span>
        {account.admin
          ? 'Administrator'
          : plural(scopedNamespaceCount(account.scopes), 'namespace')}
      </span>
      <div className="row-actions">
        <Button size="small" onClick={onEdit}>
          Edit
        </Button>
        <Button size="small" onClick={onReset}>
          Reset
        </Button>
        <Button size="small" variant="ghost" onClick={onRemove}>
          Remove
        </Button>
      </div>
    </article>
  )
}

// LocalAccountsSection manages console-managed accounts. Each change is its own request,
// so it never overwrites other settings edits.
export function LocalAccountsSection({
  settings,
  onError,
}: {
  settings: Settings
  onError: (message: string) => void
}) {
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const action = useLocalAccountAction()
  const close = () => {
    action.reset()
    setDialog(null)
  }
  return (
    <section>
      <div className="section-heading">
        <h2>Local accounts</h2>
        <div className="heading-actions">
          <Button
            onClick={() => {
              setDialog({ kind: 'add' })
            }}
          >
            <Plus size={15} />
            Add local account
          </Button>
        </div>
      </div>
      <p className="muted access-issuer">
        Local accounts sign in with a password and an authenticator app. An identity provider with
        multi-factor authentication is recommended for teams; keep a local administrator for
        recovery.
      </p>
      <LocalSignInSetting
        settings={settings}
        pending={action.isPending}
        onChange={(enabled) => {
          action.mutate({ kind: 'signIn', revision: settings.revision, enabled })
        }}
      />
      {!dialog && <FormError error={action.error} />}
      <div className="settings-list">
        {settings.localAccounts.map((account) => (
          <AccountRow
            key={account.id}
            account={account}
            onEdit={() => {
              setDialog({ kind: 'edit', account })
            }}
            onReset={() => {
              setDialog({ kind: 'reset', account })
            }}
            onRemove={() => {
              setDialog({ kind: 'remove', account })
            }}
          />
        ))}
        {settings.localAccounts.length === 0 && (
          <p className="muted local-accounts-empty">No local accounts.</p>
        )}
      </div>
      {dialog && (
        <LocalAccountDialog
          dialog={dialog}
          settings={settings}
          action={action}
          onDialog={setDialog}
          onError={onError}
          onClose={close}
        />
      )}
    </section>
  )
}

function LocalAccountDialog({
  dialog,
  settings,
  action,
  onDialog,
  onError,
  onClose,
}: {
  dialog: Dialog
  settings: Settings
  action: ReturnType<typeof useLocalAccountAction>
  onDialog: (dialog: Dialog) => void
  onError: (message: string) => void
  onClose: () => void
}) {
  const showLink = (username: string) => (result: LocalAccountResult) => {
    if (result.kind === 'link') onDialog({ kind: 'link', username, enrollment: result.enrollment })
  }
  switch (dialog.kind) {
    case 'add':
    case 'edit':
      return (
        <LocalAccountEditor
          account={dialog.kind === 'edit' ? dialog.account : null}
          configuration={settings.configuration}
          pending={action.isPending}
          error={action.error}
          onClose={onClose}
          onSave={(username, fields) => {
            if (dialog.kind === 'edit')
              action.mutate(
                { kind: 'update', revision: settings.revision, id: dialog.account.id, fields },
                { onSuccess: onClose },
              )
            else
              action.mutate(
                { kind: 'create', revision: settings.revision, username, fields },
                { onSuccess: showLink(username) },
              )
          }}
        />
      )
    case 'link':
      return (
        <EnrollmentLinkDialog
          username={dialog.username}
          enrollment={dialog.enrollment}
          onError={onError}
          onClose={onClose}
        />
      )
    case 'reset':
    case 'remove':
      return (
        <SettingsEditor
          title={`${dialog.kind === 'reset' ? 'Reset' : 'Remove'} ${dialog.account.username}?`}
          onClose={onClose}
        >
          <p>
            {dialog.kind === 'reset'
              ? 'The password, authenticator, recovery codes, and open sessions stop working now. You receive a new one-time link for the owner.'
              : 'The account, its password, authenticator, and open sessions are removed. The change history keeps its entries.'}
          </p>
          <FormError error={action.error} />
          <div className="editor-actions">
            <Button variant="ghost" onClick={onClose}>
              Cancel
            </Button>
            <Button
              variant="danger"
              disabled={action.isPending}
              onClick={() => {
                if (dialog.kind === 'reset')
                  action.mutate(
                    { kind: 'reset', id: dialog.account.id },
                    { onSuccess: showLink(dialog.account.username) },
                  )
                else
                  action.mutate(
                    { kind: 'delete', revision: settings.revision, id: dialog.account.id },
                    { onSuccess: onClose },
                  )
              }}
            >
              {dialog.kind === 'reset' ? 'Reset account' : 'Remove account'}
            </Button>
          </div>
        </SettingsEditor>
      )
  }
}
