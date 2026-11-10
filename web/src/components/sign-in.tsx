import { CircleAlert, Info, LogIn } from 'lucide-react'
import { useSignInOptions } from '../api/client'
import { BrandLogo } from './brand'
import { LocalSignInForm } from './local/local-sign-in-form'
import { useDocumentTitle } from './use-document-title'

// Bounded reasons set by the backend after a failed or ended login. Provider errors never
// reach the browser.
const notices = {
  'signed-out': { tone: 'info', text: 'You have signed out.' },
  expired: { tone: 'error', text: 'Your sign-in expired before it finished. Start again.' },
  denied: {
    tone: 'error',
    text: 'You signed in, but your account has no access to this console. Ask an administrator to enroll your user or group.',
  },
  failed: {
    tone: 'error',
    text: 'Sign-in could not be completed. Start again, and contact an administrator if this continues.',
  },
  unavailable: {
    tone: 'error',
    text: 'Sign-in is temporarily unavailable. Try again in a few minutes.',
  },
} as const

function isNotice(value: string | null): value is keyof typeof notices {
  return value !== null && Object.hasOwn(notices, value)
}

// Read once at load: the router may normalize the address before the page renders.
const result = new URLSearchParams(window.location.search).get('signin')

export function SignIn() {
  useDocumentTitle('Sign in')
  const params = new URLSearchParams(window.location.search)
  params.delete('signin')
  const query = params.toString()
  const returnTo = window.location.pathname + (query ? `?${query}` : '')
  const notice = isNotice(result) ? notices[result] : null
  const options = useSignInOptions()
  // Single sign-on stays the fallback while options load or when no local account
  // exists. Without a provider, its link returns to setup or this page.
  const local = options.data?.localAccounts ?? false
  const singleSignOn = (options.data?.singleSignOn ?? true) || !local
  return (
    <main className="sign-in">
      <div className="sign-in-panel">
        <BrandLogo className="sign-in-logo" />
        <h1>
          OpenBao Operator <span>Console</span>
        </h1>
        {notice && (
          <p
            className={`sign-in-notice sign-in-notice-${notice.tone}`}
            role={notice.tone === 'error' ? 'alert' : 'status'}
          >
            {notice.tone === 'error' ? <CircleAlert size={16} /> : <Info size={16} />}
            <span>{notice.text}</span>
          </p>
        )}
        {singleSignOn && (
          <a
            href={`/auth/login?returnTo=${encodeURIComponent(returnTo)}`}
            className="button button-primary sign-in-button"
          >
            <LogIn size={16} />
            Sign in with single sign-on
          </a>
        )}
        {singleSignOn && local && (
          <p className="sign-in-divider">
            <span>or use a local account</span>
          </p>
        )}
        {local && <LocalSignInForm returnTo={returnTo} primary={!singleSignOn} />}
      </div>
    </main>
  )
}
