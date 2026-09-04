# backup-dr Specification

## Purpose
TBD - created by archiving change add-postgres-backup-cronjob. Update Purpose after archive.

## Requirements

### Requirement: Scheduled logical backup of every service database

The platform SHALL run a Kubernetes `CronJob` that, on a configurable schedule, produces a
logical `pg_dump` of every service database on the shared Postgres instance and uploads each
dump to the MinIO/S3 object store. A single CronJob SHALL iterate the configured database list
(the eight logical databases: identity, listing/domain, search, engagement, order, payment,
chat, notification); the database list, schedule, object-store endpoint, bucket, and retention
window SHALL come from a `ConfigMap`, and all Postgres and object-store credentials SHALL come
from a `Secret` referenced by the pod (never inline in the pod spec).

#### Scenario: A scheduled run backs up all databases

- **WHEN** the CronJob fires on schedule
- **THEN** it runs `pg_dump` once per configured database and uploads each dump to the object
  store under a per-database, timestamped key (e.g. `<db>/<db>-<UTC-timestamp>.sql.gz`) in the
  configured bucket

#### Scenario: One database failing does not abort the others

- **WHEN** `pg_dump` for one database fails during a run
- **THEN** the job still attempts and uploads the remaining databases, and the job exits
  non-zero so the failure is visible to monitoring/alerting

#### Scenario: Credentials are not hard-coded

- **WHEN** the CronJob pod spec is rendered
- **THEN** the Postgres password and the object-store access/secret keys are injected from a
  Kubernetes `Secret` (via `env`/`envFrom`), and no credential literal appears in the CronJob
  or ConfigMap manifest

### Requirement: Backup retention prunes old dumps

The backup workflow SHALL enforce a retention window: after a successful upload, dumps in the
object store older than the configured retention period SHALL be deleted, so the bucket does
not grow unbounded.

#### Scenario: Dumps older than the retention window are removed

- **WHEN** a backup run completes and objects older than the configured retention days exist in
  the bucket
- **THEN** those objects are deleted and dumps within the window are retained

### Requirement: Backup target bucket is provisioned via GitOps

The backup bucket SHALL be created idempotently before backups run, and the whole backup
workload SHALL be delivered through an ArgoCD Application so it is managed declaratively like
the other platform infrastructure.

#### Scenario: ArgoCD reconciles the backup workload after its dependencies

- **WHEN** ArgoCD syncs the platform
- **THEN** an `infra-postgres-backup` Application applies the `platform/backup` manifests
  (CronJob, ConfigMap, Secret, bucket-bootstrap) in a sync-wave after Postgres and MinIO, and a
  bucket-bootstrap step ensures the backup bucket exists before the first CronJob run
