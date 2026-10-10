import { useState } from 'react'
import { APIError, useCheckConnection, useReaderAccess, useSaveSettings } from '../../api/client'
import type { Deployment, ManagedConnection, Settings } from '../../api/schema'
import { newConnection } from '../../lib/connection'
import type { CredentialDraft } from '../../lib/credential'
import {
  checkKey,
  connectionCheck,
  credentialUpdate,
  emptyCredential,
  withRenewal,
} from '../../lib/credential'
import type { Step } from '../../lib/onboarding'
import { clusterID, scopeKey, stepProblem, steps, withOperatorScope } from '../../lib/onboarding'

export interface Saved {
  id: string
  name: string
  paused: boolean
  preview: readonly Deployment[]
}

// useOnboardingWizard holds the add-cluster workflow. Reader access and check results
// apply only while the inputs they were produced for are unchanged.
export function useOnboardingWizard(
  settings: Settings,
  onReload: () => Promise<void>,
  onSaved: (saved: Saved) => void,
) {
  const [suffix] = useState(() => crypto.randomUUID().slice(0, 8))
  const [connection, setConnection] = useState(newConnection)
  const [manualID, setManualID] = useState(false)
  const [step, setStep] = useState<Step>('Cluster')
  const [draft, setDraft] = useState(() => emptyCredential('kubeconfig'))
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState('')
  const access = useReaderAccess()
  const check = useCheckConnection()
  const save = useSaveSettings()
  const scoped = withOperatorScope(connection)
  const request = connectionCheck(scoped, draft)
  const enrolled = settings.configuration.connections.map((item) => item.id)
  const stepIndex = steps.indexOf(step)
  const currentCheck =
    check.isSuccess && checkKey(check.variables) === checkKey(request) ? check.data : null

  function move(next: Step | undefined) {
    if (!next) return
    setStep(next)
    setError(null)
    setNotice('')
  }
  async function advance() {
    setError(null)
    setNotice('')
    const problem = stepProblem(step, scoped, draft, enrolled)
    if (problem) {
      setError(problem)
      return
    }
    if (step === 'Scope') {
      try {
        await access.mutateAsync(scoped)
      } catch (failure) {
        setError(failure)
        return
      }
    }
    move(steps[stepIndex + 1])
  }
  async function verify() {
    setError(null)
    await check.mutateAsync(request).catch(setError)
  }
  async function connect(paused: boolean) {
    setError(null)
    if (enrolled.includes(connection.id)) {
      setError(
        'This identifier is already enrolled. Return to Cluster and choose a different identifier.',
      )
      return
    }
    if (!paused && currentCheck?.connection.collection !== 'current') {
      setError('Verify reader access before connecting, or save this cluster paused.')
      return
    }
    const credential = credentialUpdate(request)
    try {
      await save.mutateAsync({
        revision: settings.revision,
        configuration: {
          ...settings.configuration,
          connections: [...settings.configuration.connections, { ...scoped, disabled: paused }],
        },
        credentials: credential ? [credential] : [],
      })
    } catch (failure) {
      setError(failure)
      return
    }
    const preview = currentCheck?.deployments ?? []
    onSaved({ id: connection.id, name: connection.name, paused, preview })
  }
  async function reload() {
    try {
      await onReload()
      setError(null)
      setNotice('Settings reloaded. Review the cluster and save again.')
    } catch (failure) {
      setError(failure)
    }
  }
  return {
    step,
    stepIndex,
    connection,
    draft,
    error,
    notice,
    currentAccess:
      access.isSuccess && scopeKey(access.variables) === scopeKey(scoped) ? access.data : null,
    currentCheck,
    preparing: access.isPending,
    checking: check.isPending,
    saving: save.isPending,
    conflict: error instanceof APIError && error.status === 409,
    operatorID: `operator-${suffix}`,
    setConnection,
    setDraft,
    setError,
    setNotice,
    rename: (name: string) => {
      setConnection({ ...connection, name, ...(manualID ? {} : { id: clusterID(name, suffix) }) })
    },
    identify: (id: string) => {
      setManualID(true)
      setConnection({ ...connection, id })
    },
    scope: (next: ManagedConnection) => {
      setConnection(next)
      setDraft((current: CredentialDraft) => withRenewal(current, !!next.renewal))
    },
    back: () => {
      move(steps[stepIndex - 1])
    },
    advance,
    verify,
    connect,
    reload,
  }
}
