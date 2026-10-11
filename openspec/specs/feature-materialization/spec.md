# feature-materialization Specification

## Purpose
Defines how platform-featurestore materialises versioned, point-in-time feature views from the warehouse's Parquet
exports into an offline snapshot with a manifest and versioned Redis online keys, and the parity gate between them.

## Requirements

### Requirement: The warehouse exports the feature inputs

When the Parquet export is enabled, team-analytics SHALL write `tracking_events_resolved.parquet`,
`engagement_facts.parquet` and `order_facts.parquet` beside `tracking_events.parquet` on every export cycle. Each file
SHALL be replaced atomically, so a reader never sees a partial file.

#### Scenario: A tracked view reaches the resolved export

- **WHEN** a visitor posts a view of a unique listing and the next export cycle completes
- **THEN** `tracking_events_resolved.parquet` on the analytics volume contains that listing with a `user_key`

### Requirement: Features are materialised from a versioned registry, as of a point in time

`python -m featurestore materialize` SHALL compute every feature view declared in the registry as of `AS_OF` (default:
now). The registry SHALL declare `user_activity@v2` and `item_popularity@v1`.
- `user_activity@v2`, per `user_key`:
  - views, clicks and add-to-carts in the 7 days before `AS_OF`;
  - current favourites;
  - current follows;
  - `paid_orders_30d`: the number of distinct paid orders whose `buyer_id` is the user, with `occurred_at` in the 30 days
    before `AS_OF` (after `AS_OF` minus 30 days, up to and including `AS_OF`), 0 when there are none. A user with paid
    orders but no events or facts SHALL still have a row. An order line without a `buyer_id` SHALL count for no user.
- `item_popularity@v1`, per listing:
  - views, clicks and add-to-carts in the 7 days before `AS_OF`;
  - current favourite count;
  - review count;
  - average rating;
  - 7-day click-through rate (clicks over impressions, 0 without impressions).

`user_activity@v1` is retired and SHALL NOT be materialised. The job SHALL exit with a configuration error naming the
column if `order_facts.parquet` has no `buyer_id`. Only rows with `ingested_at` at or before `AS_OF` SHALL be used (order
facts: `occurred_at` at or before `AS_OF`).

#### Scenario: A buyer's activity becomes features

- **WHEN** a new buyer views a listing three times and favourites it, the export cycle completes, and the
  materialisation job runs
- **THEN** the online features of that buyer under `user_activity@v2` have 3 views in the last 7 days and 1 current
  favourite, and the listing's `item_popularity@v1` features have at least 3 views and at least 1 current favourite

#### Scenario: Events after AS_OF are not used

- **WHEN** the job runs with `AS_OF` set to a time just before a new buyer's first event
- **THEN** that buyer has no `user_activity@v2` row in the offline snapshot of that run

#### Scenario: A paid order becomes a user feature

- **WHEN** a new buyer pays one order, the export cycle completes, and the materialisation job runs
- **THEN** the online `paid_orders_30d` of that buyer under `user_activity@v2` is 1, and that buyer has a row although
  they posted no tracking event

#### Scenario: Only orders in the 30 days before AS_OF count

- **WHEN** a user has paid orders 31 days, 29 days and 1 day before `AS_OF` and one after `AS_OF`, and the job runs
- **THEN** the user's `paid_orders_30d` is 2

#### Scenario: A multi-line order counts once

- **WHEN** a user has one paid order with three lines and one paid order with one line within the window, and the job runs
- **THEN** the user's `paid_orders_30d` is 2

#### Scenario: An order without a buyer counts for nobody

- **WHEN** the order facts hold a paid order line with no `buyer_id`, and the job runs
- **THEN** no `user_activity@v2` row has a `paid_orders_30d` that includes that order

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
- **THEN** `fs:user_activity:current` is "2" and `fs:user_activity:meta` carries the run's `as_of` and an input
  watermark no later than it

### Requirement: Online and offline features agree, or the run fails

After writing, the job SHALL compare the online values of a sample of `FEATURESTORE_PARITY_SAMPLE` entities per view
against that run's offline snapshot, and SHALL exit non-zero, naming the mismatching entities, if any value differs.
`python -m featurestore parity` SHALL run the same comparison against the latest snapshot without materialising.

#### Scenario: A tampered online value fails the parity check

- **WHEN** after a run, one buyer's `user_activity@v2` online view count is overwritten with a different value, and
  `python -m featurestore parity` runs
- **THEN** the command exits non-zero and its output names that buyer

#### Scenario: A tampered order count fails the parity check

- **WHEN** after a run, one buyer's `user_activity@v2` online `paid_orders_30d` is overwritten with a different value,
  and `python -m featurestore parity` runs
- **THEN** the command exits non-zero and its output names that buyer and `paid_orders_30d`
