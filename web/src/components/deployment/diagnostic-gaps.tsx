import type { Deployment } from '../../api/schema'
import { diagnosticReads, diagnosticReasons, diagnosticTitles } from '../../lib/diagnostic-gaps'
import { Timestamp } from '../status'

export function DiagnosticGaps({ deployment }: { deployment: Deployment }) {
  const gaps = deployment.evidence.analysisGaps ?? []
  if (!deployment.evidence.analysisIncomplete && gaps.length === 0) return null
  return (
    <div className="notice notice-warning">
      <div>
        <p>
          Some diagnostic rules could not be evaluated. Prior unresolved findings remain visible
          with their last evaluation time.
        </p>
        {gaps.length > 0 ? (
          <details>
            <summary>View incomplete checks ({gaps.length})</summary>
            <ul>
              {gaps.map((gap) => (
                <li key={`${gap.rule}:${gap.collectionOutcome}`}>
                  <strong>{diagnosticTitles[gap.rule] ?? 'Diagnostic check'}:</strong>{' '}
                  {gap.reasons.map((reason) => diagnosticReasons[reason]).join(' ')}{' '}
                  {diagnosticReads[gap.collectionOutcome]} Evaluated{' '}
                  <Timestamp value={gap.checkedAt} />.
                </li>
              ))}
            </ul>
          </details>
        ) : (
          <p>Detailed reasons are unavailable for this retained observation. Refresh collection.</p>
        )}
      </div>
    </div>
  )
}
