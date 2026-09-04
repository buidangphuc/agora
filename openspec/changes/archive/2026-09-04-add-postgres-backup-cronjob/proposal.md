## Why

The platform runs a single shared Postgres (`platform/postgres/postgres.yaml`) that
holds every service's database, but there is **no backup**: the Deployment mounts an
`emptyDir` for `/var/lib/postgresql/data`, so a pod reschedule wipes all data, and there
is nothing to restore from even in a real (PVC-backed) cluster. MinIO already runs in the
cluster (`platform/infra/minio.yaml`) as the platform's S3 object store, so a scheduled
`pg_dump` → object-store upload closes the most basic durability gap (backup-dr) with no
new infrastructure. This change is GitOps-only (`platform-gitops`); no service code changes.

## What Changes

- **platform-gitops (`platform/backup/`)** — add a Kubernetes `CronJob` that, on a schedule,
  `pg_dump`s each service database on the shared Postgres and streams each dump to MinIO/S3
  under a per-database, date-stamped object key, then prunes objects older than the retention
  window. Delivered as **raw manifests** (mirroring `platform/postgres`, `platform/infra`),
  not a new Helm chart and not via `charts/service` (that chart renders long-running
  Deployments — Service/Ingress/HPA/NetworkPolicy — and is the wrong shape for a CronJob).
- **platform-gitops (`platform/backup/`)** — add a `ConfigMap` holding the schedule, the
  explicit database list, the MinIO endpoint/bucket, and the retention days; add a `Secret`
  (or ExternalSecret stub) carrying the Postgres read credentials and the MinIO access keys
  so no credential is hard-coded in the pod spec.
- **platform-gitops (`platform/backup/`)** — add a MinIO bucket-bootstrap step (PostSync
  `mc mb --ignore-existing`, mirroring the existing `minio-init` Job) so the backup bucket
  exists before the first run.
- **platform-gitops (`argocd/apps/`)** — add `infra-postgres-backup.yaml`, an ArgoCD
  Application pointing at `platform/backup` (directory recurse), in a sync-wave after
  Postgres and MinIO so it reconciles once its dependencies are up.

### Decision — one CronJob iterating DBs (recommended), not one CronJob per DB

All service databases live on **one** Postgres instance (logical isolation only), so a single
CronJob that loops over the database list and runs `pg_dump` per DB is the right granularity:
one schedule, one Secret, one retention policy, one object to reason about — versus eight
near-identical CronJobs with no isolation benefit (they all hit the same server). The loop
dumps each DB independently, continues past a single DB's failure, and exits non-zero if **any**
DB failed so alerting/monitoring still fires. A future move to true per-DB Postgres instances
can revisit this. This, plus the credential handling below, is simple enough to fold here — **no
`design.md`**.

### Decision — credentials & the database list

- **Credentials.** `pg_dump` connects as the Postgres superuser (`postgres`, per
  `platform/postgres/postgres.yaml`) so it can dump every database; the password and the MinIO
  access/secret keys come from a Kubernetes `Secret` referenced via `envFrom`/`env` (never inline
  in the pod spec). In a real environment this Secret is the ExternalSecret/Vault seam already used
  by `charts/service` (`secrets.enabled`); locally it is a plain Secret with the known dev values.
- **Database list — naming reconciliation (key finding).** The in-cluster DB names created by
  `postgres-init` are `team_<svc>_db` (service name, hyphens→underscores) for the eight services
  `team-identity team-domain team-search team-engagement team-order team-payment team-chat
  team-notification`, whereas `docker-compose.services.yaml` uses shorter names
  (`identity_db`, `listing_db` for team-domain, `search_db`, …, and no DB for team-notification).
  The CronJob dumps what the **cluster** actually has, so its default list is the eight
  `team_<svc>_db` databases, kept in the backup `ConfigMap`. To prevent drift, the list SHOULD be
  sourced from the same `postgres-init` `SERVICES` value that creates the databases (single source
  of truth). The eight logical databases in scope: identity, listing/domain, search, engagement,
  order, payment, chat, notification.

## Non-goals

- **Restore automation / DR runbook drills** — this change only produces and retains dumps.
  Documented restore procedure and periodic restore-verification drills are a follow-up
  (`add-postgres-restore-runbook`).
- **Point-in-time recovery / WAL archiving** — `pg_dump` gives per-run snapshots only; continuous
  archiving (`archive_command`, `pg_basebackup`, PITR) is out of scope.
- **Cross-region / off-cluster replication** of the backup bucket.
- **Backups of non-Postgres state** — OpenSearch, MinIO objects, Redpanda topics are out of scope.
- **Provisioning durable storage** for Postgres itself (the `emptyDir` → PVC change is separate).
