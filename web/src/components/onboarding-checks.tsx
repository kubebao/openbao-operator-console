import type { ConnectionCheckResult, ManagedConnection } from '../api/schema'
import { capabilityCheckLabels, operatorCheckLabels } from '../lib/format'
import { Callout, DetailRows } from './page'
import { HealthBadge, Timestamp } from './status'

export function CredentialLifetime({
  expiry,
  local,
  renewal,
}: {
  expiry: ConnectionCheckResult['credentialExpiry']
  local: boolean
  renewal: boolean
}) {
  return (
    <>
      <DetailRows
        rows={[
          {
            label: 'Credential lifetime',
            value: local ? (
              'The hosting cluster manages the ServiceAccount token.'
            ) : expiry.expiresAt ? (
              <>
                {expiry.source === 'declaredToken'
                  ? 'Token-declared expiry'
                  : 'Certificate expires'}{' '}
                <Timestamp value={expiry.expiresAt} scheduled />
              </>
            ) : (
              'Unknown. Plan credential replacement with the cluster owner.'
            ),
          },
        ]}
      />
      {!local && (
        <p className="onboarding-help">
          {renewal
            ? 'Automatic renewal replaces this token after the cluster is connected. If renewal fails until the token expires, reconnect the cluster.'
            : 'This credential is not renewed automatically. Replace it in Settings before it expires.'}
          {expiry.source === 'declaredToken' &&
            ' The token expiry is an unverified declaration; the API access check verifies current acceptance.'}
        </p>
      )}
    </>
  )
}

export function OnboardingChecks({
  result,
  local,
  renewal,
}: {
  result: ConnectionCheckResult
  local: boolean
  renewal: boolean
}) {
  const optional = result.connection.capabilities.filter(
    (capability) => capability.resource !== 'openbaoclusters',
  )
  return (
    <section className="onboarding-checks" aria-label="Connection check results">
      <Callout
        tone={result.diagnostics.state === 'available' ? 'success' : 'error'}
        title={
          result.diagnostics.state === 'available'
            ? 'Reader access verified'
            : 'Connection needs attention'
        }
      >
        {result.diagnostics.message}
      </Callout>
      <CredentialLifetime expiry={result.credentialExpiry} local={local} renewal={renewal} />
      <details>
        <summary>Namespace and optional API checks</summary>
        <ul className="onboarding-result-list">
          {result.connection.capabilities.map((capability) => (
            <li key={`${capability.namespace}/${capability.resource}`}>
              <span>
                {capability.namespace} / {capability.resource}
              </span>
              <span>{capabilityCheckLabels[capability.state]}</span>
            </li>
          ))}
          {result.connection.operators.map((operator) => (
            <li key={operator.id}>
              <span>
                {operator.namespace}/{operator.name}
              </span>
              <span>{operatorCheckLabels[operator.collection]}</span>
            </li>
          ))}
        </ul>
      </details>
      {optional.some(
        (capability) => capability.state === 'denied' || capability.state === 'unsupported',
      ) && (
        <p className="onboarding-help">
          Some optional APIs are unavailable or denied. Their evidence remains unknown; basic
          deployment collection can continue.
        </p>
      )}
      <section className="onboarding-preview">
        <h3>Deployment preview</h3>
        {result.diagnostics.state !== 'available' ? (
          <p>Complete a successful check to preview deployments.</p>
        ) : result.deployments.length === 0 ? (
          <p>
            No OpenBao deployments found in the enrolled namespaces. You can connect this cluster
            and collect deployments when they appear.
          </p>
        ) : (
          <ul className="onboarding-result-list">
            {result.deployments.map((deployment) => (
              <li key={deployment.id}>
                <span>
                  <strong>{deployment.name}</strong>
                  <small>{deployment.namespace}</small>
                </span>
                <HealthBadge health={deployment.health} />
              </li>
            ))}
          </ul>
        )}
      </section>
    </section>
  )
}

// ConnectionCheckSummary reports a connection check inside the connection editor.
export function ConnectionCheckSummary({
  result,
  connection,
}: {
  result: ConnectionCheckResult
  connection: ManagedConnection
}) {
  const verified = result.diagnostics.state === 'available'
  return (
    <section className="connection-check">
      <Callout
        tone={verified ? 'success' : 'error'}
        title={verified ? 'Reader access verified' : 'Connection needs attention'}
      >
        {result.diagnostics.message}
      </Callout>
      <CredentialLifetime
        expiry={result.credentialExpiry}
        local={connection.authentication === 'local'}
        renewal={!!connection.renewal}
      />
      <ul>
        {result.connection.capabilities.map((capability) => (
          <li key={`${capability.namespace}/${capability.resource}`}>
            {capability.namespace} / {capability.resource}
            <span>{capabilityCheckLabels[capability.state]}</span>
          </li>
        ))}
        {result.connection.operators.map((operator) => (
          <li key={operator.id}>
            {operator.namespace}/{operator.name}
            <span>{operatorCheckLabels[operator.collection]}</span>
          </li>
        ))}
      </ul>
      <details>
        <summary>Reader RBAC manifests</summary>
        <textarea aria-label="Reader RBAC manifests" readOnly rows={8} value={result.rbac} />
      </details>
    </section>
  )
}
