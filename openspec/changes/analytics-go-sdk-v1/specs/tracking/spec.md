## MODIFIED Requirements

### Requirement: Events are written in batches for throughput

The worker SHALL accumulate tracking events and flush them to the warehouse in batches (target
throughput ~100 events/second), bounded by a batch size and a max flush interval so that a partial
batch is not held indefinitely. A flush failure SHALL NOT silently drop events: the batch is attempted 5 times, and offsets are
committed only after a successful write or after the batch is parked on `KAFKA_DLQ_TOPIC` following 5 failed attempts. A parked record keeps its key and headers and carries `ga_client_id` only when `consent.analytics` is true.

#### Scenario: A burst of events is written as batches

- **WHEN** a burst of tracking events arrives on `analytics.events`
- **THEN** the batch handler returns nil only after the writer holds the tracking rows of the batch, and returns the writer's error unchanged when a write fails, so the SDK does not commit a batch that was neither written nor parked (the broker-side commit is proven by "A batch that cannot be written is parked, not dropped" and "A 1000-event burst is written at 100 events/s or faster")
- **VERIFIED BY**: team-analytics `internal/events` `TestBatchHandlerWritesTrackingAndOrderFactsThenAcknowledges`

#### Scenario: A batch that cannot be written is parked, not dropped

- **WHEN** the warehouse write of a batch fails on all 5 attempts
- **THEN** every record of the batch is on `KAFKA_DLQ_TOPIC` with its original key and value (`ga_client_id` blanked when the record has no analytics consent) before its offset is committed, and the next batch is processed
- **VERIFIED BY**: team-analytics `internal/app` `TestPoisonBatchIsDeadLetteredAfterFiveAttempts`

#### Scenario: A 1000-event burst is written at 100 events/s or faster

- **WHEN** 1000 tracking events are produced to `analytics.events` on a Redpanda broker in one burst and the consumer runs on a DuckDB file in a temporary directory with the default `BATCH_MAX_SIZE` (500) and `BATCH_FLUSH_INTERVAL_SECONDS` (2)
- **THEN** the warehouse holds 1000 tracking rows within 10 s of the first produce (at least 100 events/s) and the group offset on `analytics.events` equals the produced end offset
- **VERIFIED BY**: team-analytics `internal/app` `TestBurstOf1000EventsIsWrittenAtOrAbove100PerSecond`
