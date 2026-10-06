import type { Deployment, Issue } from '../api/schema'

export type DeploymentTab = 'overview' | 'conditions' | 'operations' | 'backup' | 'history'

// Evidence rows that a finding can point to. Each value names one rendered detail row.
export type EvidenceField =
  | 'replicas'
  | 'sealed'
  | 'collection'
  | 'replicaHistory'
  | 'upgrade'
  | 'recovery'
  | 'backupFailures'
  | 'backupSuccess'
  | 'restoreTest'

interface FindingLocation {
  tab: DeploymentTab
  fields: readonly EvidenceField[]
}

// Rule identifiers come from internal/evidence/rules.go. Unknown rules stay on the overview,
// where every finding is listed.
const locations: Record<string, FindingLocation> = {
  'replica-readiness': { tab: 'overview', fields: ['replicas'] },
  sealed: { tab: 'conditions', fields: ['sealed'] },
  collection: { tab: 'overview', fields: ['collection'] },
  'replica-history-conflict': { tab: 'overview', fields: ['replicaHistory'] },
  'upgrade-failed': { tab: 'operations', fields: ['upgrade'] },
  'recovery-investigate': { tab: 'operations', fields: ['recovery'] },
  'backup-failed': { tab: 'backup', fields: ['backupFailures'] },
  'backup-never-succeeded': { tab: 'backup', fields: ['backupSuccess'] },
  'backup-overdue': { tab: 'backup', fields: ['backupSuccess'] },
  'restore-test': { tab: 'backup', fields: ['restoreTest'] },
}

export function findingLocation(issue: Pick<Issue, 'rule'>): FindingLocation {
  return locations[issue.rule] ?? { tab: 'overview', fields: [] }
}

const severityRank: Record<Issue['severity'], number> = { critical: 0, high: 1, info: 2 }

function strongest(issues: readonly Issue[]): Issue['severity'] | null {
  let result: Issue['severity'] | null = null
  for (const issue of issues)
    if (result === null || severityRank[issue.severity] < severityRank[result])
      result = issue.severity
  return result
}

export function tabSeverity(
  deployment: Pick<Deployment, 'issues'>,
  tab: DeploymentTab,
): Issue['severity'] | null {
  return strongest(deployment.issues.filter((issue) => findingLocation(issue).tab === tab))
}

export function fieldSeverity(
  deployment: Pick<Deployment, 'issues'>,
  field: EvidenceField,
): Issue['severity'] | null {
  return strongest(
    deployment.issues.filter((issue) => findingLocation(issue).fields.includes(field)),
  )
}
