# Tasks

## 1. Code — platform-gitops (`platform/backup/`)
- [x] `platform/backup/configmap.yaml` (new): `pg-backup-config` ConfigMap — `SCHEDULE`
      (e.g. `0 2 * * *`), `DATABASES` (default: the eight `team_<svc>_db` names, sourced from
      `postgres-init` `SERVICES`), `PGHOST=postgres.marketplace.svc`, `PGPORT=5432`,
      `S3_ENDPOINT=http://minio.marketplace.svc:9000`, `S3_BUCKET=postgres-backups`,
      `RETENTION_DAYS` (e.g. `7`), and the `backup.sh` loop script.
- [x] `platform/backup/secret.yaml` (new): `pg-backup-secret` — `PGUSER=postgres`,
      `PGPASSWORD` (matches `platform/postgres/postgres.yaml`), `S3_ACCESS_KEY=minioadmin`,
      `S3_SECRET_KEY=minioadmin`. (In prod this becomes an ExternalSecret over Vault, mirroring
      `charts/service` `secrets.enabled`.)
- [x] `platform/backup/cronjob.yaml` (new): `postgres-backup` CronJob reading `SCHEDULE`; single
      container (image with `pg_dump` + `mc`/`aws`), `envFrom` the ConfigMap + Secret; `backup.sh`
      loops the `DATABASES` list, `pg_dump | gzip | mc pipe` per DB to
      `<bucket>/<db>/<db>-<ts>.sql.gz`, continues on per-DB failure, tracks a failure flag, then
      `mc rm --recursive --force --older-than "${RETENTION_DAYS}d"` and exits non-zero if any DB
      failed. `concurrencyPolicy: Forbid`, sane `backoffLimit`/history limits.
- [x] `platform/backup/bucket-init.yaml` (new): PostSync `Job` (mirror `minio-init` in
      `platform/infra/minio.yaml`) — `mc alias set … && mc mb --ignore-existing local/postgres-backups`.
- [x] `argocd/apps/infra-postgres-backup.yaml` (new): ArgoCD Application, `path: platform/backup`,
      `directory.recurse: true`, `namespace: marketplace`, sync-wave after Postgres+MinIO
      (e.g. `"2"`), `automated { prune, selfHeal }`, `CreateNamespace=true`.

## 2. Verify — render & dry-run (no service code)
- [x] `kubectl apply --dry-run=client -f platform/backup/` accepts every manifest.
- [x] CronJob schema valid (`batch/v1` CronJob, valid cron expr); Secret referenced only via
      `env`/`envFrom` — grep the pod spec for inline credential literals (must be none).
- [x] Dry-run the loop locally against the compose stack: `pg_dump` each DB → `mc cp` to MinIO →
      confirm objects land under `postgres-backups/<db>/…`; re-run and confirm `--older-than`
      prune removes an aged object. (No cluster required; uses the local Postgres + MinIO.)

## 3. E2E (platform-e2e)
- [x] No user-facing capability and no gRPC/UI surface → **no `FEATURES.yaml` change and no
      `.feature` scenario** (per the E2E contract, FEATURES.yaml tracks user-facing capabilities;
      this is platform infra like `platform/postgres`). RECORDED: backup-dr is verified by the
      render/dry-run + the local dump/prune integration run in section 2 (ephemeral Postgres+MinIO),
      not by pytest-bdd. VERIFIED end-to-end: `backup.sh` (extracted verbatim from the ConfigMap)
      dumped `team_identity_db`+`team_domain_db` to `postgres-backups/<db>/<db>-<UTC-ts>.sql.gz`;
      a missing `team_search_db` failed but the loop continued and the job exited 1; the exact
      `mc rm --older-than 7d` retained recent dumps while `--older-than 0d` pruned them; happy path
      (all DBs present) printed `backup complete` (exit 0). mc image tag corrected to `minio/mc:latest`
      (matching the existing gitops `minio-init`; the compose file's pinned `RELEASE.*` tag no longer resolves).
- [ ] `make -C platform-e2e features-check` remains green (unchanged — this change adds no feature).
      NOTE: not run here; no FEATURES.yaml change.

## 4. Archive
- [ ] Confirm the ArgoCD app syncs and a manual CronJob trigger uploads all eight dumps + prunes.
      NOTE: cluster-dependent (no live cluster). Client dry-run of all `platform/backup/` manifests +
      the ArgoCD Application is clean; the loop is proven against ephemeral Postgres+MinIO (section 3).
      Actual in-cluster CronJob run across all eight `team_<svc>_db` deferred to a real cluster.
- [ ] Note the follow-up `add-postgres-restore-runbook` (restore automation + drills) is filed.
- [ ] Archive the change (`/opsx:archive add-postgres-backup-cronjob`).
