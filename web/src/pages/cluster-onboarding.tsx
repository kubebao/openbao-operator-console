import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { AlertCircle, ArrowLeft, Lock } from 'lucide-react'
import { useSettings, useSnapshot } from '../api/client'
import type { Settings } from '../api/schema'
import { FormError } from '../components/form'
import { ClusterStep } from '../components/onboarding/cluster-step'
import { ConnectedSummary } from '../components/onboarding/connected-summary'
import { CredentialsStep } from '../components/onboarding/credentials-step'
import { ScopeStep } from '../components/onboarding/scope-step'
import { StepProgress } from '../components/onboarding/step-progress'
import { VerifyStep } from '../components/onboarding/verify-step'
import { OnboardingReader } from '../components/onboarding-reader'
import { EmptyState, PageHeading } from '../components/page'
import { useDocumentTitle } from '../components/use-document-title'
import { Button } from '../components/ui/button'
import { formError } from '../lib/errors'
import type { Saved } from '../components/onboarding/use-wizard'
import { useOnboardingWizard } from '../components/onboarding/use-wizard'

export function ClusterOnboardingPage() {
  const { data: snapshot } = useSnapshot()
  const query = useSettings(snapshot?.viewer.admin === true)
  const [saved, setSaved] = useState<Saved | null>(null)
  useDocumentTitle('Add cluster')
  if (snapshot && !snapshot.viewer.admin)
    return (
      <div className="page-enter">
        <PageHeading title="Add cluster" />
        <EmptyState title="Administrator access required" icon={Lock}>
          A console administrator can add clusters.
        </EmptyState>
      </div>
    )
  // A failed reload keeps the wizard and its typed credentials; only a failed first load blocks.
  if (query.isError && query.data === undefined)
    return (
      <div className="page-enter">
        <PageHeading title="Add cluster" />
        <EmptyState title="Settings unavailable" icon={AlertCircle}>
          {formError(query.error)}{' '}
          <Button
            onClick={() => {
              void query.refetch()
            }}
          >
            Reload settings
          </Button>
        </EmptyState>
      </div>
    )
  if (!query.data)
    return (
      <div className="page-enter">
        <PageHeading title="Add cluster" />
        <p className="loading-state">Loading settings…</p>
      </div>
    )
  if (saved) return <ConnectedSummary {...saved} />
  return (
    <OnboardingWizard
      settings={query.data}
      reloading={query.isRefetching}
      onReload={async () => {
        const result = await query.refetch()
        if (result.isError) throw result.error
      }}
      onSaved={setSaved}
    />
  )
}

function OnboardingWizard({
  settings,
  reloading,
  onReload,
  onSaved,
}: {
  settings: Settings
  reloading: boolean
  onReload: () => Promise<void>
  onSaved: (saved: Saved) => void
}) {
  const navigate = useNavigate()
  const [commandContext, setCommandContext] = useState('')
  const wizard = useOnboardingWizard(settings, onReload, onSaved)
  const { step, connection } = wizard
  const pending = wizard.preparing || wizard.checking || wizard.saving || reloading
  return (
    <div className="page-enter onboarding-layout">
      <PageHeading
        title="Add cluster"
        actions={
          <Button
            variant="ghost"
            onClick={() => {
              void navigate({ to: '/settings', search: { section: 'connections' } })
            }}
          >
            Cancel
          </Button>
        }
      />
      <StepProgress step={step} />
      <form
        className="onboarding-workspace"
        onSubmit={(event) => {
          event.preventDefault()
          void wizard.advance()
        }}
      >
        <fieldset disabled={pending} className="onboarding-content">
          <legend className="sr-only">{step}</legend>
          {step === 'Cluster' && (
            <ClusterStep
              connection={connection}
              onName={wizard.rename}
              onID={wizard.identify}
              onChange={wizard.setConnection}
            />
          )}
          {step === 'Scope' && (
            <ScopeStep
              connection={connection}
              operatorID={wizard.operatorID}
              onChange={wizard.scope}
            />
          )}
          {step === 'Reader access' && (
            <OnboardingReader
              access={wizard.currentAccess}
              local={connection.authentication === 'local'}
              context={commandContext}
              onContext={setCommandContext}
              onNotice={wizard.setNotice}
              onError={wizard.setError}
            />
          )}
          {step === 'Credentials' && (
            <CredentialsStep
              connection={connection}
              draft={wizard.draft}
              onConnection={wizard.setConnection}
              onDraft={wizard.setDraft}
              onError={wizard.setError}
            />
          )}
          {step === 'Verify' && (
            <VerifyStep
              connection={connection}
              result={wizard.currentCheck}
              checking={wizard.checking}
              onCheck={() => {
                void wizard.verify()
              }}
            />
          )}
        </fieldset>
        <FormError error={wizard.error}>
          {wizard.conflict && (
            <>
              <p>Reload the latest settings before saving. Your cluster details stay here.</p>
              <Button
                type="button"
                disabled={pending}
                onClick={() => {
                  void wizard.reload()
                }}
              >
                Reload settings
              </Button>
            </>
          )}
        </FormError>
        {wizard.notice && (
          <p className="save-notice" role="status">
            {wizard.notice}
          </p>
        )}
        <div className="onboarding-actions">
          {wizard.stepIndex > 0 && (
            <Button type="button" variant="ghost" disabled={pending} onClick={wizard.back}>
              <ArrowLeft size={15} />
              Back
            </Button>
          )}
          {step === 'Verify' ? (
            <>
              <Button
                type="button"
                disabled={pending}
                onClick={() => {
                  void wizard.connect(true)
                }}
              >
                Save paused
              </Button>
              <Button
                type="button"
                variant="default"
                disabled={pending || wizard.currentCheck?.connection.collection !== 'current'}
                onClick={() => {
                  void wizard.connect(false)
                }}
              >
                {wizard.saving ? 'Saving…' : 'Connect cluster'}
              </Button>
            </>
          ) : (
            <Button type="submit" variant="default" disabled={pending}>
              {wizard.preparing ? 'Preparing…' : 'Continue'}
            </Button>
          )}
        </div>
      </form>
    </div>
  )
}
