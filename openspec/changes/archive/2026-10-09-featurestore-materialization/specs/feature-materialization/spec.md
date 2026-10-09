## ADDED Requirements

### Requirement: The warehouse exports the feature inputs

When the Parquet export is enabled, team-analytics SHALL write `tracking_events_resolved.parquet`,
`engagement_facts.parquet` and `order_facts.parquet` beside `tracking_events.parquet` on every export cycle. Each file
SHALL be replaced atomically, so a reader never sees a partial file.

#### Scenario: A tracked view reaches the resolved export

- **WHEN** a visitor posts a view of a unique listing and the next export cycle completes
- **THEN** `tracking_events_resolved.parquet` on the analytics volume contains that listing with a `user_key`

### Requirement: Features are materialised from a versioned registry, as of a point in time

`python -m featurestore materialize` SHALL compute every feature view declared in the registry as of `AS_OF` (default:
now). The registry SHALL declare `user_activity@v1` and `item_popularity@v1`.
- `user_activity@v1`, per `user_key`:
  - views, clicks and add-to-carts in the 7 days before `AS_OF`;
  - current favourites;
  - current follows.

  `order_facts` carries no buyer, so per-user order counts are not part of v1; they need a buyer column in the
  warehouse first and then a new view version.
- `item_popularity@v1`, per listing:
  - views, clicks and add-to-carts in the 7 days before `AS_OF`;
  - current favourite count;
  - review count;
  - average rating;
  - 7-day click-through rate (clicks over impressions, 0 without impressions).

Only rows with `ingested_at` at or before `AS_OF` SHALL be used.

#### Scenario: A buyer's activity becomes features

- **WHEN** a new buyer views a listing three times and favourites it, the export cycle completes, and the
  materialisation job runs
- **THEN** the online features of that buyer under `user_activity@v1` have 3 views in the last 7 days and 1 current
  favourite, and the listing's `item_popularity@v1` features have at least 3 views and at least 1 current favourite

#### Scenario: Events after AS_OF are not used

- **WHEN** the job runs with `AS_OF` set to a time just before a new buyer's first event
- **THEN** that buyer has no `user_activity@v1` row in the offline snapshot of that run

### Requirement: Each run writes an offline snapshot and a manifest

Each run SHALL write one Parquet snapshot per view and version under
`<offline dir>/<view>/v<version>/as_of=<as_of>.parquet`, and one `manifest.json` per run under
`<offline dir>/runs/<as_of>/`. The manifest SHALL record:
- `as_of`;
- the row count per view;
- the SHA-256 of each view's SQL definition;
- the input watermark, which is the latest `ingested_at` read.

Snapshots of earlier runs SHALL be kept.

#### Scenario: A run leaves a snapshot and a manifest

- **WHEN** the materialisation job runs twice with different `AS_OF` values
- **THEN** two snapshots exist for each view, and each manifest names its `as_of`, a positive row count and a 64-hex
  definition hash

### Requirement: Online features are versioned and carry their freshness

The job SHALL write each entity's features to Redis under `fs:<view>:v<version>:<entity_id>` as JSON, with
`FEATURESTORE_ONLINE_TTL_SECONDS`. After every entity is written, it SHALL set `fs:<view>:current` to the version and
`fs:<view>:meta` to the run's `as_of`, `materialized_at` and input watermark.

#### Scenario: The online store says how fresh it is

- **WHEN** the materialisation job finishes
- **THEN** `fs:user_activity:current` is "1" and `fs:user_activity:meta` carries the run's `as_of` and an input
  watermark no later than it

### Requirement: Online and offline features agree, or the run fails

After writing, the job SHALL compare the online values of a sample of `FEATURESTORE_PARITY_SAMPLE` entities per view
against that run's offline snapshot, and SHALL exit non-zero, naming the mismatching entities, if any value differs.
`python -m featurestore parity` SHALL run the same comparison against the latest snapshot without materialising.

#### Scenario: A tampered online value fails the parity check

- **WHEN** after a run, one buyer's `user_activity@v1` online value is overwritten with a different view count, and
  `python -m featurestore parity` runs
- **THEN** the command exits non-zero and its output names that buyer
