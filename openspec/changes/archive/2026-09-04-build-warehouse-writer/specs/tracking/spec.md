## ADDED Requirements

### Requirement: A consumer sinks tracking events from analytics.events into the warehouse

The system SHALL provide a `team-analytics` worker that consumes the Kafka topic `analytics.events`
with a consumer group, unmarshals each `platform.events.v1.EventEnvelope` whose
`type = "platform.analytics.v1.TrackingEvent"` into a `TrackingEvent`, and appends it to the
analytics warehouse. Envelopes of any other `type` SHALL be skipped without error, and the worker
SHALL own its warehouse storage exclusively (DB-per-service, Rule 3).

#### Scenario: A produced tracking event lands in the warehouse

- **WHEN** an `EventEnvelope` carrying a `TrackingEvent` is produced to `analytics.events`
- **THEN** the worker consumes it, unmarshals the payload, and the event's behavioral fields
  (event type, listing id, session id, page/path, occurred-at) plus the envelope principal appear
  as a row in the warehouse

#### Scenario: Non-tracking envelopes are ignored

- **WHEN** an envelope whose `type` is not `platform.analytics.v1.TrackingEvent` appears on the topic
- **THEN** the worker skips it without writing a row and without failing the consumer

### Requirement: The warehouse target is swappable behind a WarehouseWriter seam

The worker SHALL write through a single `WarehouseWriter` interface with interchangeable adapters
selected by `WAREHOUSE_DRIVER`: `duckdb` (local/test, columnar Parquet) and `bigquery` (prod).
Switching drivers SHALL require no change to the consume/unmarshal path — only the env value.

#### Scenario: Local runs use the DuckDB adapter

- **WHEN** the worker starts with `WAREHOUSE_DRIVER=duckdb`
- **THEN** it writes tracking rows to the local DuckDB/Parquet warehouse, and the same rows would be
  written to BigQuery under `WAREHOUSE_DRIVER=bigquery` with no change to event handling

### Requirement: Events are written in batches for throughput

The worker SHALL accumulate tracking events and flush them to the warehouse in batches (target
throughput ~100 events/second), bounded by a batch size and a max flush interval so that a partial
batch is not held indefinitely. A flush failure SHALL NOT silently drop events — offsets are
committed only after a successful write.

#### Scenario: A burst of events is written as batches

- **WHEN** a burst of tracking events arrives on `analytics.events`
- **THEN** the worker flushes them to the warehouse in batches and commits consumer offsets only
  after the batch is durably written, so a crash mid-batch reprocesses rather than loses events
