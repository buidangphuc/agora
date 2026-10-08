## Purpose

Defines how `team-analytics` produces the offline store (history, for training) and the online store (latest
value, for serving) from the same registered definition, and the guarantee that both agree. It covers what a
feature value at a given time means, when each store is written, and what happens when materialization fails.

## ADDED Requirements

### Requirement: A feature value at time t uses only what was knowable at t

The system SHALL compute the value of a feature version as of a time `t` from exactly the event rows that had
been received by the warehouse at or before `t` (ingestion time) and whose occurrence time lies inside the
feature's window ending at `t`. Duplicate deliveries of the same event SHALL count once. Rows of service
principals SHALL NOT contribute to behavioural features.

#### Scenario: An event received after t does not change the value at t

- **WHEN** a click on listing L occurred at 10:00:00 but reached the warehouse at 10:03:00
- **THEN** `item.clicks_7d@v1` for L as of 10:02:00 does not count it, and as of 10:03:00 does

#### Scenario: An event outside the window does not count

- **WHEN** a view of listing L occurred 8 days before `t`
- **THEN** `item.views_7d@v1` for L as of `t` does not count it

#### Scenario: A redelivered event counts once

- **WHEN** the same tracking event (same event id) is delivered and stored twice
- **THEN** every feature value counts it once

### Requirement: The offline store is written from the registered SQL as partitions with a manifest

The system SHALL run each materialized feature version's SQL in a batch at a fixed set of as-of times per day
(the version's offline grain) and write the results as Parquet under
`features/offline/<entity>.<name>@v<N>/dt=YYYY-MM-DD/` in object storage, then write `_manifest.json` for that
partition last, carrying the schema version, the feature reference, the SQL hash, the as-of times, the input
watermark, the row count and a content checksum. A partition without a manifest SHALL be treated as absent.
Re-running a partition with the same inputs SHALL produce the same rows; floating-point feature values SHALL be stored rounded to 12 significant digits so the result does not depend on row order or thread split. Consent and erasure state are inputs read as of the re-run, not as of the partition's as-of time: a re-run after a subject opted out or was erased omits that subject.

#### Scenario: A daily partition is published with its manifest last

- **WHEN** the batch materializer completes the partition `dt=2026-10-01` of `item.views_7d@v1`
- **THEN** the Parquet files and then `_manifest.json` exist under that prefix, and the manifest's SQL hash
  equals the registry's hash for `item.views_7d@v1`

#### Scenario: A half-written partition is invisible

- **WHEN** the batch materializer stops after writing Parquet files but before the manifest
- **THEN** the dataset builder and `DescribeFeatures` treat the partition as absent, and the next run
  rewrites it

#### Scenario: Backfill reproduces history

- **WHEN** an operator backfills `item.views_7d@v2` for the last 30 days
- **THEN** one partition per day is written with the same rows a daily run on that day would have produced
  from the same retained events

### Requirement: The online store is written from the same SQL on a short interval

The system SHALL run the same registered SQL every micro-batch interval (default 60 s) for every entity whose
window changed since the previous run (new events, or events leaving the window) and for time-decayed features
at least once per offline grain, and SHALL write each result into the hash `fs:v1:<entity>:<id>`, field
`<name>@v<N>`, with the as-of time and SQL hash stored beside it. Each written key SHALL carry an expiry at
least as long as the longest TTL of its fields. A computed zero SHALL be written as zero; an entity with no
value SHALL have no field.

#### Scenario: A click becomes visible online within the freshness SLO

- **WHEN** a click on listing L is produced to `analytics.events`
- **THEN** within the freshness SLO of `item.clicks_7d@v1` the hash `fs:v1:item:L` carries an increased
  `clicks_7d@v1` and an as-of time after the click's ingestion time

#### Scenario: A value drops when its events leave the window

- **WHEN** the only views of listing L fall out of the 1-day window and no new view arrives
- **THEN** a later micro-batch writes `views_1d@v1 = 0` for L with the new as-of time

#### Scenario: The online materializer is down

- **WHEN** the online materializer stops running
- **THEN** the last written values stay readable until their TTL and are reported stale afterwards, and
  freshness alerts fire (see feature-monitoring)

### Requirement: Offline and online values agree, enforced as a hard gate

The system SHALL provide a parity check that, for a sample of entities and features, compares the value the
online materializer wrote at time `t` with the offline computation of the same feature version as of `t`. The
merge gate of `team-analytics` SHALL run it on a fixed synthetic event stream and fail on any mismatch (exact
for integer and list types, relative tolerance 1e-9 for floating types). The running service SHALL also sample
parity continuously and expose the mismatch count as a metric.

#### Scenario: Parity holds on the synthetic stream

- **WHEN** the merge gate replays the synthetic event stream through both materializers
- **THEN** every sampled (entity, feature, t) pair has equal offline and online values and the gate passes

#### Scenario: A divergent online path fails the gate

- **WHEN** the online path is changed so that it computes any feature differently from its registered SQL
  (for example it drops the duplicate-event rule)
- **THEN** the parity check reports the feature, entity and both values, and the merge gate fails

#### Scenario: A version cannot become stable without parity

- **WHEN** a change sets a version to `stable` and its parity check has not passed
- **THEN** the merge gate fails naming the version
