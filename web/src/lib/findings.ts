import type { Deployment, Issue } from '../api/schema'

// Deployment page tabs in display order. The router validates the tab search value with it.
export const deploymentTabs = ['overview', 'lifecycle', 'backups', 'conditions', 'history'] as const
export type DeploymentTab = (typeof deploymentTabs)[number]

export const tabLabels: Record<DeploymentTab, string> = {
  overview: 'Overview',
  lifecycle: 'Lifecycle',
  backups: 'Backups',
  conditions: 'Conditions',
  history: 'History',
}

// Evidence rows that a finding can point to. Each value names one rendered detail row.
export type EvidenceField =
  | 'replicas'
  | 'availability'
  | 'conditions'
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
  availability: { tab: 'conditions', fields: ['availability'] },
  'operator-condition': { tab: 'conditions', fields: ['conditions'] },
  sealed: { tab: 'conditions', fields: ['sealed'] },
  collection: { tab: 'overview', fields: ['collection'] },
  'replica-history-conflict': { tab: 'lifecycle', fields: ['replicaHistory'] },
  'upgrade-failed': { tab: 'lifecycle', fields: ['upgrade'] },
  'recovery-investigate': { tab: 'lifecycle', fields: ['recovery'] },
  'backup-failed': { tab: 'backups', fields: ['backupFailures'] },
  'backup-never-succeeded': { tab: 'backups', fields: ['backupSuccess'] },
  'backup-overdue': { tab: 'backups', fields: ['backupSuccess'] },
  'restore-test': { tab: 'backups', fields: ['restoreTest'] },
}

export function findingLocation(issue: Pick<Issue, 'rule'>): FindingLocation {
  return locations[issue.rule] ?? { tab: 'overview', fields: [] }
}

// Lower ranks sort first: critical findings lead.
export const severityRank: Record<Issue['severity'], number> = { critical: 0, high: 1, info: 2 }

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

// A condition finding highlights only the row that supplied its evidence. Older sealed
// findings have no conditionType and keep their original row association.
export function conditionSeverity(
  deployment: Pick<Deployment, 'issues'>,
  conditionType: string,
): Issue['severity'] | null {
  return strongest(
    deployment.issues.filter(
      (issue) =>
        issue.conditionType === conditionType ||
        (issue.rule === 'sealed' && conditionType === 'OpenBaoSealed'),
    ),
  )
}

// Overview tiles summarize groups of evidence rows.
export type EvidenceDimension = 'availability' | 'backups' | 'restoreTest' | 'lifecycle'

const dimensions: Record<EvidenceDimension, readonly EvidenceField[]> = {
  availability: ['replicas', 'sealed', 'availability', 'conditions'],
  backups: ['backupFailures', 'backupSuccess'],
  restoreTest: ['restoreTest'],
  lifecycle: ['upgrade', 'recovery', 'replicaHistory'],
}

export function dimensionSeverity(
  deployment: Pick<Deployment, 'issues'>,
  dimension: EvidenceDimension,
): Issue['severity'] | null {
  return strongest(
    deployment.issues.filter((issue) =>
      findingLocation(issue).fields.some((field) => dimensions[dimension].includes(field)),
    ),
  )
}
