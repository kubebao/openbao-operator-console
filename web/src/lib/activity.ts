import type { Activity, FindingRule } from '../api/activity'
import type { Issue } from '../api/schema'
import type { Change } from './changes'
import { collectionChange, healthChange } from './changes'
import { severityRank } from './findings'

// Rule names match the finding titles of internal/evidence/rules.go.
export const ruleLabels: Record<FindingRule, string> = {
  'replica-readiness': 'Replicas need attention',
  sealed: 'OpenBao is sealed',
  'upgrade-failed': 'Upgrade requires investigation',
  'backup-failed': 'Backup failed',
  'backup-never-succeeded': 'No successful backup observed',
  'backup-overdue': 'Backup is overdue',
  'restore-test': 'Restore test needs attention',
  'replica-history-conflict': 'Replica history conflict',
  'recovery-investigate': 'Recovery requires investigation',
  collection: 'Collection is incomplete',
}

const severityLabels: Record<Issue['severity'], string> = {
  critical: 'Critical',
  high: 'High',
  info: 'Info',
}

type Finding = NonNullable<Activity['finding']>

function findingTone(severity: Issue['severity']): Change['tone'] {
  return severity === 'critical' ? 'critical' : 'warning'
}

function severityChange(finding: Finding): Change {
  const previous = finding.previousSeverity ?? finding.severity
  const raised = severityRank[finding.severity] < severityRank[previous]
  return {
    title: `Finding ${raised ? 'escalated' : 'eased'}: ${ruleLabels[finding.rule]}`,
    detail: `Severity ${severityLabels[previous]} → ${severityLabels[finding.severity]}`,
    tone: raised ? findingTone(finding.severity) : 'neutral',
  }
}

// The response schema guarantees that the change matching the kind is present.
const missing: Change = { title: 'Change recorded', tone: 'neutral' }

// describeActivity renders one structured activity event with the history wording.
export function describeActivity(event: Activity): Change {
  const finding = event.finding
  switch (event.kind) {
    case 'legacy':
      return {
        title: 'Earlier activity recorded',
        detail: 'This older record has no structured change details.',
        tone: 'neutral',
      }
    case 'healthChanged':
      return event.health ? healthChange(event.health.from, event.health.to) : missing
    case 'collectionChanged':
      return event.collection
        ? collectionChange(event.collection.from, event.collection.to)
        : missing
    case 'findingRaised':
      return finding
        ? {
            title: `Finding raised: ${ruleLabels[finding.rule]}`,
            detail: `Severity ${severityLabels[finding.severity]}`,
            tone: findingTone(finding.severity),
          }
        : missing
    case 'findingResolved':
      return finding
        ? { title: `Finding resolved: ${ruleLabels[finding.rule]}`, tone: 'success' }
        : missing
    case 'findingSeverityChanged':
      return finding ? severityChange(finding) : missing
  }
}
