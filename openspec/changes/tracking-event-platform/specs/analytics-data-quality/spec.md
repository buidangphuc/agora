## Purpose

Defines the data-quality gate over ingested events: schema drift, null rates, duplicate rate and volume anomalies
against a trailing baseline, exposed as metrics, alerts and a per-table status that the feature store reads to
decide whether data is fresh and trustworthy enough to materialize.

## ADDED Requirements

### Requirement: Data-quality checks run on a schedule per table and event type

`team-analytics` SHALL evaluate, at least every 15 minutes over the last complete hour, per table and per event type:
schema drift (unknown fields, newer `schema_version`, type mismatches), null rate of each required field, duplicate
rate (deliveries dropped by `event_id` dedupe divided by deliveries), quarantine rate, and volume compared with the
same hour over a trailing 7-day baseline. Each check SHALL have configurable warn and fail thresholds.

#### Scenario: A missing attribution field is caught

- **WHEN** a frontend bug sends impressions with an empty `model_version` for one hour, so its null rate for
  `impression` jumps above the fail threshold
- **THEN** the next run marks `tracking_events/impression` as `fail` with check `null_rate:model_version`

#### Scenario: A volume drop is caught

- **WHEN** impressions in the last hour are below 30% of the trailing baseline for that hour
- **THEN** the run marks `tracking_events/impression` as `fail` with check `volume_anomaly`

#### Scenario: A quiet start does not alert

- **WHEN** fewer than 7 days of history exist for an event type
- **THEN** the volume check reports `insufficient_baseline` and does not fail

### Requirement: Quality results are exposed as metrics, alerts and a readable status

Each run SHALL export Prometheus metrics (per table/type: check value, status, last run time, ingestion watermark
lag), SHALL persist the run, and SHALL expose the latest status per table/type (`pass`, `warn`, `fail`) and the
ingestion watermark through the admin data-quality RPC. Alert rules SHALL fire on `fail` and on watermark lag above
its threshold. A downstream materializer SHALL be able to read the status and watermark to apply its freshness SLO;
the feature store does so in-process, so the RPC SHALL be admin-only and SHALL deny service principals.

#### Scenario: An admin reads freshness and status; a service principal is denied

- **WHEN** an admin asks for the data-quality report
- **THEN** it receives, per table and event type, the latest status, the failing checks and the ingestion watermark
- **AND** a service principal asking for the same report receives PERMISSION_DENIED

#### Scenario: A failing check raises an alert

- **WHEN** a table/type is `fail` for two consecutive runs
- **THEN** the `AnalyticsDataQualityFailing` alert fires with the table, type and check labels
