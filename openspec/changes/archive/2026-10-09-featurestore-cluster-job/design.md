## Context

See proposal.md. Constraints found in the repo: raw manifests under `platform/<name>/` are synced by an Application with
`directory.recurse` (pattern: `platform-recsys`); infra has no Secret needs here (valkey has no auth); the compose
services are the contract for env (`FEATURESTORE_*`, `AS_OF`) and command order.

## Decisions

1. **One CronJob, sequential containers.** `materialize` and `parity` are init containers, `dataset` is the main
   container. Kubernetes runs init containers in order and stops at the first failure, so a parity mismatch (exit 3)
   or registry drift (exit 4) means no dataset is built that night. `materialize` already gates on parity; the
   explicit `parity` step re-reads the latest manifest as the spec asks. Alternative (three CronJobs with offsets)
   rejected: no ordering guarantee.
2. **Schedule.** Featurestore `15 2 * * *`, recsys stays `0 3 * * *`. The analytics export runs every 300 s, so the
   input is at most minutes old. `concurrencyPolicy: Forbid`, `startingDeadlineSeconds: 600`.
3. **Shared volume.** RWO PVC `featurestore-data` (1Gi): written by the featurestore, mounted `readOnly: true` by
   recsys. ArgoCD `prune: true` would delete a PVC removed from Git, so the PVC lives in the featurestore Application.
4. **Input volume.** team-analytics currently keeps `/data` on an `emptyDir` and has no `PARQUET_EXPORT_PATH`, so no
   Parquet is exported in the cluster. It moves to PVC `analytics-data` with the export enabled (300 s), mirroring
   compose. Featurestore mounts it read-only.
5. **Fail closed on stale.** recsys code only handles the absent case (exit 2). An init container
   (`busybox`, `find <DATASET_DIR> -name 'as_of=*.parquet' -mmin -<MAX_DATASET_AGE_MINUTES>`) exits non-zero when no
   snapshot is that fresh or the directory is missing. Default 1500 min (25 h) for a nightly cadence.
6. **Hardening.** Pod: `runAsNonRoot`, uid/gid 10001, `fsGroup` 10001, seccomp RuntimeDefault. Container: no privilege
   escalation, all caps dropped, read-only root filesystem with an `emptyDir` on `/tmp` (`HOME=/tmp`).
   NetworkPolicy selects `app: featurestore`, no ingress, egress to kube-dns (53) and `app: valkey` (6379).
7. **No secrets.** Valkey needs none; env is plain values in a ConfigMap. If auth is added later, use the repo's
   ExternalSecret pattern (`<svc>-env`), not a literal.

## Risks

- RWO + node affinity on multi-node clusters (see proposal Impact).
- Moving analytics-data to a PVC: first sync starts with an empty store, same as today's `emptyDir`.
- Cannot be verified without a cluster: the scheduler running the three steps, PVC binding and the NetworkPolicy being
  enforced by the CNI. The static scenarios cover the rendered shape only.
