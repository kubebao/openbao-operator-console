import type { Deployment } from '../api/schema'

export type DiagnosticGap = NonNullable<Deployment['evidence']['analysisGaps']>[number]

export const diagnosticTitles: Record<string, string> = {
  'replica-readiness': 'Replica readiness',
  availability: 'Operator availability',
  'operator-condition': 'Operator conditions',
  sealed: 'Seal state',
  'upgrade-failed': 'Upgrade evidence',
  'backup-failed': 'Backup attempts',
  'backup-never-succeeded': 'Backup success',
  'backup-overdue': 'Backup age',
  'restore-test': 'Restore testing',
  'recovery-investigate': 'Recovery evidence',
}

export const diagnosticReasons: Record<DiagnosticGap['reasons'][number], string> = {
  excluded: 'The evaluation policy did not select this check.',
  not_requested: 'The required evidence was not requested.',
  scope_incomplete: 'Required reads did not complete.',
  missing_observation: 'A required observation is missing.',
  stale_evidence: 'The collected evidence is too old for this check.',
  missing_field: 'A required field is missing, invalid, or unrecognized.',
  generation_unconfirmed: 'Operator acknowledgement does not match the resource generation.',
  inconsistent_time: 'A reported timestamp is inconsistent with collection time.',
  not_configured: 'The required feature is not configured.',
  higher_priority_backup_signal: 'Another backup check covers the reported failure.',
  target_unverified: 'A later read did not verify the target identity and generation.',
  association_unconfirmed: 'Related resource ownership is not confirmed by UID.',
  scope_restricted: 'The selected read scope does not cover this check.',
  unknown: 'No detailed diagnostic reason was recorded.',
}

export const diagnosticReads: Record<DiagnosticGap['collectionOutcome'], string> = {
  not_requested: 'Read not requested. Check the configured collection scope.',
  not_collected: 'Read not collected. Wait for collection to complete.',
  complete: 'Read completed; the evidence still does not support this check.',
  partial: 'Only part of the requested scope was collected.',
  denied: 'Read access denied. Check the enrolled reader permissions.',
  expired: 'The read expired. Retry collection.',
  rate_limited: 'The API rate limited the read. Retry collection later.',
  unsupported: 'The requested API is unsupported. Check the installed CRDs and API group.',
  timeout: 'The read timed out. Check API connectivity.',
  cancelled: 'The read was cancelled before completion.',
  page_limit: 'The read reached its page limit.',
  size_limit: 'The read reached its size limit.',
  pagination_loop: 'The API returned a repeated pagination token.',
  deleted: 'The target was deleted. Refresh inventory.',
  replaced: 'The target UID changed. Refresh inventory.',
  invalid: 'The API returned evidence that could not be validated.',
  failed: 'The read failed. Check connection status.',
  unknown: 'The read outcome was not recorded.',
}
