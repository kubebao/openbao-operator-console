import { AlertCircle, CheckCircle2, CircleHelp, Info, ListChecks, RefreshCw } from 'lucide-react'
import type { Deployment } from '../../api/schema'
import type { ConditionAssessment } from '../../lib/conditions'
import { assessConditions, assessmentLabels } from '../../lib/conditions'
import { fieldSeverity } from '../../lib/findings'
import type { Observation } from '../../lib/history'
import { conditionTransitions } from '../../lib/history'
import { EmptyState } from '../page'
import { Timestamp } from '../status'
import { SectionTitle } from '../info-tip'

const icons: Record<ConditionAssessment, typeof Info> = {
  problem: AlertCircle,
  unknown: CircleHelp,
  active: RefreshCw,
  informational: Info,
  normal: CheckCircle2,
}

export function ConditionsTab({
  deployment,
  observations,
}: {
  deployment: Deployment
  observations: readonly Observation[] | null
}) {
  const views = assessConditions(deployment)
  const transitions = observations
    ? conditionTransitions(observations)
    : new Map<string, { from: string; at: string }>()
  if (views.length === 0)
    return (
      <EmptyState title="No conditions reported" icon={ListChecks}>
        The operator has not published status conditions for this resource.
      </EmptyState>
    )
  const sealedFinding = fieldSeverity(deployment, 'sealed')
  return (
    <section className="detail-section">
      <SectionTitle
        tipLabel="About conditions"
        tip="Interpretation follows the operator's condition types. Unrecognized types are shown as reported. Conditions from an older resource generation are not current evidence."
      >
        Operator conditions
      </SectionTitle>
      <ul className="condition-list">
        {views.map(({ condition, assessment, outdated }) => {
          const Icon = icons[assessment]
          const transition = transitions.get(condition.type)
          const finding = condition.type === 'OpenBaoSealed' ? sealedFinding : null
          return (
            <li
              key={condition.type}
              className={`condition condition-${assessment}${finding ? ` finding-${finding}` : ''}`}
            >
              <Icon size={18} aria-label={assessmentLabels[assessment]} />
              <div className="condition-main">
                <div className="condition-title">
                  <strong>{condition.type}</strong>
                  <span className={`condition-status status-${condition.status.toLowerCase()}`}>
                    {condition.status}
                  </span>
                  {condition.reason && <code>{condition.reason}</code>}
                  {outdated && (
                    <span
                      className="badge status-unknown"
                      title="The operator has not reported this condition for the current resource generation. Attention rules ignore it."
                    >
                      Outdated · generation {condition.observedGeneration} of{' '}
                      {deployment.generation}
                    </span>
                  )}
                </div>
                {condition.message && <p>{condition.message}</p>}
              </div>
              <div className="condition-time">
                {transition ? (
                  <>
                    <span>
                      {transition.from} → {condition.status}
                    </span>
                    <Timestamp value={transition.at} />
                  </>
                ) : (
                  <>
                    <span>Last transition</span>
                    <Timestamp value={condition.lastTransitionAt} missing="Not reported" />
                  </>
                )}
              </div>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
