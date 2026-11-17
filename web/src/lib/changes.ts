import type { Deployment } from '../api/schema'
import type { ConditionAssessment } from './conditions'
import { assessCondition } from './conditions'
import {
  bytes,
  cleanupStates,
  collectionLabels,
  healthLabels,
  recoveryPhases,
  restoreOutcomes,
} from './format'

export type ChangeTone = 'critical' | 'warning' | 'success' | 'neutral'

export interface Change {
  title: string
  detail?: string
  tone: ChangeTone
}

type Backup = NonNullable<Deployment['backup']>
type Recovery = NonNullable<Deployment['recovery']>
type RestoreTest = NonNullable<Backup['restoreTest']>

const healthTone: Record<Deployment['health'], ChangeTone> = {
  critical: 'critical',
  attention: 'warning',
  healthy: 'success',
  unknown: 'neutral',
}

const assessmentTone: Record<ConditionAssessment, ChangeTone> = {
  problem: 'warning',
  normal: 'success',
  unknown: 'neutral',
  active: 'neutral',
  informational: 'neutral',
}

const restoreTone: Record<RestoreTest['outcome'], ChangeTone> = {
  unknown: 'neutral',
  passed: 'success',
  failed: 'warning',
  blocked: 'warning',
  running: 'neutral',
  notRun: 'neutral',
}

const recoveryTone: Record<Recovery['phase'], ChangeTone> = {
  completed: 'success',
  failed: 'warning',
  unknown: 'warning',
  pending: 'neutral',
  validating: 'neutral',
  running: 'neutral',
}

function replicas(value: number | null): string {
  return value === null ? 'unknown' : String(value)
}

type Health = Deployment['health']
type Collection = Deployment['evidence']['collection']

// healthChange and collectionChange describe a transition in history and in activity alike.
export function healthChange(from: Health, to: Health): Change {
  return { title: `State ${healthLabels[from]} → ${healthLabels[to]}`, tone: healthTone[to] }
}

export function collectionChange(from: Collection, to: Collection): Change {
  return {
    title: `Collection ${collectionLabels[from]} → ${collectionLabels[to]}`,
    tone: to === 'current' ? 'success' : 'warning',
  }
}

// Evaluation times change each poll; only recorded causes describe a coverage change.
function gapCauses(deployment: Deployment) {
  return (deployment.evidence.analysisGaps ?? []).map((gap) => ({
    rule: gap.rule,
    reasons: gap.reasons,
    collectionOutcome: gap.collectionOutcome,
  }))
}

function stateChanges(previous: Deployment, current: Deployment): Change[] {
  const changes: Change[] = []
  if (previous.health !== current.health)
    changes.push(healthChange(previous.health, current.health))
  const before = previous.evidence.collection
  const after = current.evidence.collection
  if (before !== after) changes.push(collectionChange(before, after))
  if (previous.observedGeneration !== current.observedGeneration)
    changes.push({
      title: 'Resource acknowledgement updated',
      detail: `Observed generation ${replicas(previous.observedGeneration)} → ${replicas(current.observedGeneration)}`,
      tone: 'neutral',
    })
  if (
    previous.evidence.analysisIncomplete !== current.evidence.analysisIncomplete ||
    JSON.stringify(gapCauses(previous)) !== JSON.stringify(gapCauses(current))
  )
    changes.push({ title: 'Recorded diagnostic coverage changed', tone: 'neutral' })
  return changes
}

function findingChanges(previous: Deployment, current: Deployment): Change[] {
  const before = new Map(previous.issues.map((issue) => [issue.id, issue]))
  const after = new Map(current.issues.map((issue) => [issue.id, issue]))
  const raised = [...after.values()]
    .filter((issue) => !before.has(issue.id))
    .map((issue): Change => ({
      title: `Finding raised: ${issue.title}`,
      detail: issue.detail,
      tone: issue.severity === 'critical' ? 'critical' : 'warning',
    }))
  const resolved = [...before.values()]
    .filter((issue) => !after.has(issue.id))
    .map((issue): Change => ({ title: `Finding resolved: ${issue.title}`, tone: 'success' }))
  return [...raised, ...resolved]
}

function specificationChanges(previous: Deployment, current: Deployment): Change[] {
  const changes: Change[] = []
  if (previous.desiredReplicas !== current.desiredReplicas)
    changes.push({
      title: `Desired replicas ${replicas(previous.desiredReplicas)} → ${replicas(current.desiredReplicas)}`,
      tone: 'neutral',
    })
  if (previous.readyReplicas !== current.readyReplicas) {
    const ready =
      current.readyReplicas !== null &&
      current.desiredReplicas !== null &&
      current.readyReplicas >= current.desiredReplicas
    changes.push({
      title: `Ready replicas ${replicas(previous.readyReplicas)} → ${replicas(current.readyReplicas)}`,
      tone: ready ? 'success' : 'warning',
    })
  }
  if (previous.desiredVersion !== current.desiredVersion)
    changes.push({
      title: `Version requested: ${previous.desiredVersion || 'none'} → ${current.desiredVersion || 'none'}`,
      tone: 'neutral',
    })
  if (previous.version !== current.version)
    changes.push({
      title: `Running version ${previous.version || 'unknown'} → ${current.version || 'unknown'}`,
      tone: current.version === current.desiredVersion ? 'success' : 'neutral',
    })
  if (previous.generation !== current.generation)
    changes.push({
      title: 'Specification changed',
      detail: `Generation ${String(previous.generation)} → ${String(current.generation)}`,
      tone: 'neutral',
    })
  return changes
}

type Condition = Deployment['conditions'][number]

// A condition whose status is unchanged reports a new reason, or a transition that
// changed and changed back between two collections.
function unchangedStatus(old: Condition, condition: Condition): Change | null {
  if (old.reason !== condition.reason)
    return {
      title: `${condition.type} reason ${old.reason || 'none'} → ${condition.reason || 'none'}`,
      tone: 'neutral',
    }
  if (old.lastTransitionAt === condition.lastTransitionAt) return null
  return {
    title: `${condition.type} changed and returned to ${condition.status}`,
    ...(condition.reason ? { detail: condition.reason } : {}),
    tone: 'neutral',
  }
}

function conditionChanges(previous: Deployment, current: Deployment): Change[] {
  const changes: Change[] = []
  const conditions = new Map(previous.conditions.map((condition) => [condition.type, condition]))
  for (const condition of current.conditions) {
    const old = conditions.get(condition.type)
    if (old?.status === condition.status) {
      const change = unchangedStatus(old, condition)
      if (change) changes.push(change)
      continue
    }
    changes.push({
      title: old
        ? `${condition.type} ${old.status} → ${condition.status}`
        : `${condition.type} reported ${condition.status}`,
      ...(condition.message ? { detail: condition.message } : {}),
      tone: assessmentTone[assessCondition(condition, current.generation).assessment],
    })
  }
  const remaining = new Set(current.conditions.map((condition) => condition.type))
  for (const condition of previous.conditions)
    if (!remaining.has(condition.type))
      changes.push({ title: `${condition.type} no longer reported`, tone: 'neutral' })
  return changes
}

// Counters such as consecutive backup failures can change by more than one between
// observations, so their entries describe the observed change, not individual attempts.
function backupChanges(previous: Deployment, current: Deployment): Change[] {
  const changes: Change[] = []
  const old: Backup | null = previous.backup
  const backup = current.backup
  if (!backup) return changes
  if (backup.lastSuccessAt && backup.lastSuccessAt !== old?.lastSuccessAt)
    changes.push({
      title: 'Backup completed',
      detail: [backup.sizeBytes === null ? null : bytes(backup.sizeBytes), backup.key]
        .filter(Boolean)
        .join(' · '),
      tone: 'success',
    })
  else if (backup.nextAt && old && backup.nextAt !== old.nextAt)
    changes.push({ title: 'Backup schedule updated', tone: 'neutral' })
  const added =
    backup.consecutiveFailures !== null && old?.consecutiveFailures != null
      ? backup.consecutiveFailures - old.consecutiveFailures
      : 0
  if (added > 0)
    changes.push({
      title: added === 1 ? 'Backup failed' : `${String(added)} backup failures observed`,
      detail: `${String(backup.consecutiveFailures)} consecutive ${backup.consecutiveFailures === 1 ? 'failure' : 'failures'}`,
      tone: 'warning',
    })
  return changes
}

function restoreTestChanges(previous: Deployment, current: Deployment): Change[] {
  const old = previous.backup?.restoreTest ?? null
  const test = current.backup?.restoreTest ?? null
  if (!test?.finishedAt || test.finishedAt === old?.finishedAt) return []
  return [
    {
      title: `Restore test ${restoreOutcomes[test.outcome].toLocaleLowerCase()}`,
      ...(test.reason ? { detail: test.reason } : {}),
      tone: restoreTone[test.outcome],
    },
  ]
}

function upgradeChanges(previous: Deployment, current: Deployment): Change[] {
  if (current.upgradeFailure && !previous.upgradeFailure)
    return [{ title: `Upgrade failed: ${current.upgradeFailure.reason}`, tone: 'warning' }]
  if (!current.upgradeFailure && previous.upgradeFailure)
    return [{ title: 'Upgrade failure cleared', tone: 'success' }]
  return []
}

function recoveryChanges(previous: Deployment, current: Deployment): Change[] {
  const old = previous.recovery
  const recovery = current.recovery
  if (!recovery) return []
  const changes: Change[] = []
  if (recovery.requestName !== old?.requestName)
    changes.push({
      title: `Recovery requested: ${recovery.requestName}`,
      detail: `Phase ${recoveryPhases[recovery.phase]}`,
      tone: 'neutral',
    })
  else if (recovery.phase !== old.phase)
    changes.push({
      title: `Recovery ${recoveryPhases[old.phase]} → ${recoveryPhases[recovery.phase]}`,
      tone: recoveryTone[recovery.phase],
    })
  if (old && recovery.cleanup !== old.cleanup)
    changes.push({
      title: `Recovery cleanup ${cleanupStates[recovery.cleanup].toLocaleLowerCase()}`,
      tone: recovery.cleanup === 'failed' ? 'warning' : 'neutral',
    })
  return changes
}

function replicaHistoryChanges(previous: Deployment, current: Deployment): Change[] {
  const before = previous.replica?.historyConflict === true
  const after = current.replica?.historyConflict === true
  if (after && !before) return [{ title: 'Replica history conflict recorded', tone: 'critical' }]
  if (before && !after) return [{ title: 'Replica history conflict cleared', tone: 'success' }]
  return []
}

// Each describer reports one evidence area. The order defines the order of a history entry.
const describers: readonly ((previous: Deployment, current: Deployment) => Change[])[] = [
  stateChanges,
  findingChanges,
  specificationChanges,
  conditionChanges,
  backupChanges,
  restoreTestChanges,
  upgradeChanges,
  recoveryChanges,
  replicaHistoryChanges,
]

// describeChange lists what changed between two consecutive observations of a deployment.
export function describeChange(previous: Deployment, current: Deployment): Change[] {
  return describers.flatMap((describe) => describe(previous, current))
}
