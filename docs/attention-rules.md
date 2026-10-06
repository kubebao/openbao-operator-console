# Attention rules

The console evaluates each collected OpenBao deployment against a fixed set of rules. A rule that matches creates a
finding with a severity. Findings appear in **Attention**, mark the matching tab on the deployment page, and highlight
the evidence rows that triggered them.

Rules use current evidence only. When collection fails, the console keeps the last known findings and adds a collection
finding instead of treating missing evidence as recovery.

## Deployment state

| State | Meaning |
| --- | --- |
| Critical | At least one critical finding. |
| Attention | At least one high finding and no critical finding. |
| Healthy | Ready replicas meet desired replicas and no finding applies. |
| Unknown | Replica counts are missing or desired replicas are zero, and no finding applies. |

The Fleet view sorts by state in that order: critical, attention, unknown, then healthy.

## Rules

| Finding | Severity | Category | Triggered when | Evidence shown on |
| --- | --- | --- | --- | --- |
| Replicas need attention | Critical when 0 ready, otherwise high | Service availability | Ready replicas are below desired replicas after the startup grace period | Overview: ready / desired replicas |
| OpenBao is sealed | Critical | Service availability | The operator reports `OpenBaoSealed=True` for the current resource generation | Conditions tab |
| Upgrade requires investigation | High | Operations | The operator records an upgrade failure | Operations tab |
| Recovery requires investigation | High | Operations | A recovery request is failed or unknown, or its cleanup failed | Operations tab |
| Replica history conflict | Critical | Operations | A remote replica observation records conflicting histories | Overview: replica history |
| Backup failed | High | Backup protection | One or more consecutive backup attempts failed | Backup evidence tab |
| No successful backup observed | High | Backup protection | Backups are configured but no successful completion is recorded | Backup evidence tab |
| Backup is overdue | High | Backup protection | The last successful backup is older than the maximum backup age | Backup evidence tab |
| Restore test needs attention | High | Backup protection | The latest restore test failed or is blocked | Backup evidence tab |
| Collection is incomplete | High | Collection | The console cannot refresh this deployment's evidence | Overview: collection result |

Paused clusters do not raise replica or sealed findings. Conditions observed for an older resource generation are
ignored.

## Policies

Administrators set two thresholds in **Settings → Policies**:

| Policy | Default | Effect |
| --- | --- | --- |
| Maximum backup age | 24 hours | A configured backup older than this raises **Backup is overdue**. |
| Startup grace | 10 minutes | Replica findings wait this long after a deployment is created. |

## Collection and freshness

The collector reads every enrolled cluster every 30 seconds. A connection without a completed collection for two minutes
becomes stale. Stale and failed collections keep the last known deployments and findings, mark them as uncertain, and
add **Collection is incomplete**.

The Attention page also shows banners when a cluster cannot provide current evidence or an enrolled operator Deployment
is unavailable, progressing, or not observed. Deployments in an unreachable cluster cannot be discovered, so the
console cannot assess them.

## What findings do not prove

The console reports what the operator and Kubernetes record. It does not test OpenBao itself.

- Ready replicas are workload readiness, not a successful client transaction.
- The absence of findings means no rule matched current evidence. It is not a health guarantee.
- Backup completion age is not a measured recovery point objective (RPO). A past upload does not prove the backup is
  still readable.
- A restore-test pass covers a pinned snapshot restored into a disposable target in the same cluster. It does not test
  client logins, secret reads, or a cross-cluster recovery.
- Operator Deployment readiness does not prove successful reconciliation, and the version label does not verify the
  running binary.
- The console has no general controller heartbeat. Controller observation freshness is shown as unknown.
