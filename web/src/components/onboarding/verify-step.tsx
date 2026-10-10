import type { ConnectionCheckResult, ManagedConnection } from '../../api/schema'
import { OnboardingChecks } from '../onboarding-checks'
import { DetailRows } from '../page'
import { Button } from '../ui/button'

function verifyLabel(checking: boolean, checked: boolean): string {
  if (checking) return 'Checking…'
  return checked ? 'Check again' : 'Verify connection'
}

export function VerifyStep({
  connection,
  result,
  checking,
  onCheck,
}: {
  connection: ManagedConnection
  result: ConnectionCheckResult | null
  checking: boolean
  onCheck: () => void
}) {
  const local = connection.authentication === 'local'
  const renewal = connection.renewal
    ? `Automatic, as ${connection.renewal.namespace}/${connection.renewal.serviceAccount}`
    : 'Manual replacement'
  return (
    <>
      <h2>Verify {connection.name}</h2>
      <p className="onboarding-help">
        Checks reader access and previews deployments before saving this cluster.
      </p>
      <DetailRows
        rows={[
          {
            label: 'Namespaces',
            value: connection.namespaces.map((namespace) => namespace.name).join(', '),
          },
          {
            label: 'Reader',
            value: local ? 'Hosting cluster ServiceAccount' : 'Uploaded reader credential',
          },
          ...(local ? [] : [{ label: 'Credential renewal', value: renewal }]),
        ]}
      />
      <div className="verify-action">
        <Button type="button" variant={result ? 'outline' : 'default'} onClick={onCheck}>
          {verifyLabel(checking, result !== null)}
        </Button>
        {!result && !checking && (
          <span className="muted">Nothing is saved until you connect the cluster.</span>
        )}
      </div>
      {result && <OnboardingChecks result={result} local={local} renewal={!!connection.renewal} />}
    </>
  )
}
