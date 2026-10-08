## Purpose

Defines how a model obtains training data: it declares a versioned feature view, `team-analytics` builds a
point-in-time correct dataset from the offline store, and the trainer reads only that dataset. It exists to
make leakage and training/serving skew structurally impossible rather than a matter of care.

## ADDED Requirements

### Requirement: A consumer declares what it needs in a versioned view file

The system SHALL accept a view file owned by the consuming repository (for example
`platform-recsys/feature_views/als_interactions@v1.yaml`) that declares: the view name and version, the
entity, the pinned feature references (`<entity>.<name>@v<N>`), the label references (registry entries of kind
`label`) if any, the spine (`snapshot` at one as-of time, or `events` from a label definition), the time range
and the split (train and evaluation cut-off times). A view SHALL reference only registered, non-retired,
non-pending versions; it SHALL NOT contain SQL.

#### Scenario: A valid view is accepted

- **WHEN** a build is requested for `als_interactions@v1`, which pins `user_item.implicit_score_decayed@v1`
  with a snapshot spine and a train cut-off
- **THEN** the build is accepted and gets a build id

#### Scenario: A view pinning a pending or retired version is refused

- **WHEN** a view pins `item.orders_30d@v1` while that version is `pending`, or a version past its sunset date
- **THEN** the build is refused with `FAILED_PRECONDITION` naming the reference and its status

#### Scenario: A view with an unknown reference fails the consumer's merge gate

- **WHEN** a consumer's view file names a reference that the registry does not contain
- **THEN** the consumer's view validation test fails before the change merges

### Requirement: Datasets are point-in-time correct

The system SHALL attach to each spine row the latest offline value of each pinned feature whose as-of time is
at or before the row's timestamp, by an as-of join, and SHALL never use a value computed after it. For a
snapshot spine the value SHALL be computed by the registered SQL as of the snapshot time itself. A spine row
with no earlier value SHALL carry an explicit missing indicator for that feature, not zero. Rows of the
evaluation split SHALL use feature values as of their own timestamps, and every label SHALL be computed only
from events after the train cut-off for the evaluation split.

#### Scenario: A feature value from after the label time is not used

- **WHEN** a spine row for item L has timestamp 2026-09-14 12:00 and offline values of `item.views_7d@v1`
  exist as of 2026-09-14 00:00 (value 250) and 2026-09-15 00:00 (value 40)
- **THEN** the dataset row carries 250

#### Scenario: No value before the label time is explicit

- **WHEN** a spine row's timestamp is earlier than the first offline value of a pinned feature for its entity
- **THEN** the dataset row carries a null value and the feature's missing indicator set, never zero

#### Scenario: A time-based split does not leak

- **WHEN** a view declares train cut-off T1 and evaluation range (T1, T2]
- **THEN** every training row's features and labels are computed only from events ingested at or before T1,
  and every evaluation label only from events in (T1, T2]

### Requirement: A dataset is published as Parquet with a manifest, reproducibly

The system SHALL write each build to `features/datasets/<view>@v<N>/<build_id>/` as Parquet and then
`_manifest.json` last, carrying the view hash, every pinned reference with its SQL hash, the spine and split
times, the offline partitions used, row counts per split and a content checksum. Two builds of the same view
version over the same retained partitions SHALL have the same content checksum. A build SHALL be refused when
an offline partition it needs is absent or failed its quality gate.

#### Scenario: A finished build has a complete manifest

- **WHEN** a dataset build for `als_interactions@v1` completes
- **THEN** `GetDatasetBuild` reports it done with the manifest path, and the manifest lists the view hash,
  `user_item.implicit_score_decayed@v1` with its SQL hash, and the row counts per split

#### Scenario: A rebuild is byte-for-byte reproducible

- **WHEN** the same view version is built twice over the same retained partitions
- **THEN** both manifests carry the same content checksum

#### Scenario: A build over a missing partition is refused

- **WHEN** the requested range includes a day whose offline partition has no manifest
- **THEN** the build ends in a failed state naming the feature and the missing day, and no manifest is
  written

### Requirement: Trainers read only datasets, never raw events

The system SHALL make the dataset manifest the only training input a consumer of the feature store reads. A
consumer's merge gate SHALL fail when its training code reads the raw tracking events (the warehouse table, its
Parquet export or the DuckDB file) or computes a registered feature itself, and the trainer SHALL refuse a
dataset whose manifest view hash differs from its own view file.

#### Scenario: Training code that reads raw events fails the gate

- **WHEN** a change to `platform-recsys` adds a read of `tracking_events`, of a `WAREHOUSE_PARQUET_PATH` file
  or of a DuckDB file in its training path
- **THEN** the raw-read guard test of `platform-recsys` fails naming the file and line

#### Scenario: The trainer refuses a dataset built from another view

- **WHEN** the dataset client is handed a manifest whose view hash does not match the local
  `als_interactions@v1.yaml`
- **THEN** it raises an error and the trainer does not start
