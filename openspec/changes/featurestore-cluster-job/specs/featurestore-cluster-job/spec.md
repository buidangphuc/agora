## ADDED Requirements

### Requirement: The cluster runs the featurestore before recsys trains

The GitOps repo SHALL define a `featurestore` CronJob that runs, in one pod and in this order, `materialize`, `parity`
and `dataset`, a step starting only after the previous one succeeded. The CronJob SHALL be scheduled earlier in the day
than the recsys CronJob, SHALL forbid concurrent runs, and SHALL be synced by an ArgoCD Application registered under
`argocd/apps/`.

#### Scenario: The three steps run in order

- **WHEN** the featurestore CronJob manifest is rendered
- **THEN** its init containers run `materialize` then `parity`, and its main container runs `dataset`, all from the
  `platform-featurestore` image, with `concurrencyPolicy` `Forbid`

#### Scenario: The featurestore is scheduled before recsys

- **WHEN** the featurestore and recsys CronJobs are rendered
- **THEN** the featurestore schedule fires at an earlier time of day than the recsys schedule, and an Application
  syncs `platform/featurestore`

### Requirement: Recsys reads the featurestore dataset from a shared read-only volume

The featurestore CronJob SHALL write its offline output to a persistent volume claim, and the recsys CronJob SHALL mount
that same claim read-only at the parent of its `DATASET_DIR`, instead of an `emptyDir`. The featurestore SHALL read the
team-analytics Parquet exports from a volume it mounts read-only, which team-analytics writes via `PARQUET_EXPORT_PATH`.

#### Scenario: The dataset volume is shared

- **WHEN** the manifests are rendered
- **THEN** the featurestore mounts claim `featurestore-data` read-write at `/features`, recsys mounts the same claim
  read-only so that `DATASET_DIR` lies under its mount, and recsys has no `emptyDir` volume for it

#### Scenario: The analytics export reaches the featurestore

- **WHEN** the manifests are rendered
- **THEN** team-analytics sets `PARQUET_EXPORT_PATH` under its `/data` mount, which is claim `analytics-data`, and the
  featurestore mounts `analytics-data` read-only at `FEATURESTORE_INPUT_DIR`

### Requirement: Recsys fails closed on an absent or stale dataset

The recsys CronJob SHALL not start training when the dataset directory is missing, has no snapshot, or its newest
snapshot is older than `MAX_DATASET_AGE_MINUTES`.

#### Scenario: Absent dataset blocks training

- **WHEN** the recsys freshness guard runs against a directory that does not exist, or one with no snapshot
- **THEN** it exits non-zero

#### Scenario: Stale dataset blocks training

- **WHEN** the guard runs against a directory whose only snapshot is older than `MAX_DATASET_AGE_MINUTES`
- **THEN** it exits non-zero

#### Scenario: Fresh dataset passes the guard

- **WHEN** the guard runs against a directory holding a snapshot newer than `MAX_DATASET_AGE_MINUTES`
- **THEN** it exits zero

### Requirement: The featurestore job is least-privilege and holds no secrets

The featurestore CronJob SHALL run as non-root with a read-only root filesystem, no privilege escalation and all
capabilities dropped; SHALL declare CPU and memory requests and limits on every container; SHALL be covered by a
NetworkPolicy that allows no ingress and egress only to DNS and valkey; and SHALL carry no credential literal in Git.

#### Scenario: The job is hardened and isolated

- **WHEN** the featurestore manifests are rendered
- **THEN** every container has the security context and resources above, a NetworkPolicy selecting `app: featurestore`
  has no ingress rules and its egress peers are only DNS and `app: valkey`, and no `Secret` kind or secret-looking env
  literal is present
