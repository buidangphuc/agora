# tracking Specification

## Purpose

The tracking capability defines the platform's analytics event contract: behavioral events
(product view, click, add-to-cart, search-result impression) modeled once in
`platform-core/packages/proto` and carried on a dedicated Kafka stream, separate from the
transactional per-service databases. Tracking events reuse `platform.events.v1.EventEnvelope`
(the domain payload wrapped and switched on its fully-qualified `type`) and ride the
`analytics.events` topic. This stream is the source for the data warehouse (DuckDB local /
BigQuery prod) and the downstream recommendation engine. Producers and consumers are defined by
later changes; this capability is the contract they depend on.

## Requirements

### Requirement: Analytics tracking events are defined in the contract

The platform contract SHALL define a `platform.analytics.v1.TrackingEvent` message in
`platform-core/packages/proto/platform/analytics/v1/analytics.proto`, distinguishing at least the
event types VIEW, CLICK, ADD_TO_CART, and IMPRESSION via an `EventType` enum whose zero value is
`EVENT_TYPE_UNSPECIFIED` (values prefixed `EVENT_TYPE_*` per buf `ENUM_VALUE_PREFIX`). The message
SHALL carry the behavioral context needed for warehousing and recommendation (listing id, session
id, anonymous id, page/path, referrer, result position, search query) plus a
`map<string,string> properties` extension field, and SHALL NOT carry authenticated user identity in
its own fields (identity travels in the envelope's `principal`).

#### Scenario: Contract defines the four tracking event types

- **WHEN** a producer needs to record a product view, click, add-to-cart, or search impression
- **THEN** it can construct a single `platform.analytics.v1.TrackingEvent` with the corresponding
  `EventType`, without any new message type per event

### Requirement: Tracking events reuse the standard event envelope and a dedicated topic

The system SHALL transport tracking events using the existing `platform.events.v1.EventEnvelope`
(the `TrackingEvent` serialized into `payload`, with `type = "platform.analytics.v1.TrackingEvent"`),
on the dedicated Kafka topic **`analytics.events`** — separate from `listing.events` and from any
transactional database. The contract change SHALL be additive and non-breaking.

#### Scenario: Tracking event rides the shared envelope on its own topic

- **WHEN** the contract is generated in a consumer repo and a `TrackingEvent` is wrapped in an
  `EventEnvelope`
- **THEN** the envelope's `type` field identifies it as `platform.analytics.v1.TrackingEvent`, the
  payload round-trips (marshal/unmarshal) unchanged, and the agreed transport topic is `analytics.events`

#### Scenario: Contract passes lint and breaking-change checks

- **WHEN** `buf lint` and `buf breaking` run against `platform-core/packages/proto`
- **THEN** both pass — the new `platform.analytics.v1` package introduces no lint violations and no
  breaking change to any existing package

### Requirement: The edge collector accepts browsing beacons and produces tracking events

The gateway SHALL expose a lightweight telemetry endpoint (`POST /api/track`) that accepts a
browser beacon describing a browsing action of type VIEW, CLICK, ADD_TO_CART, or IMPRESSION, maps
it to a `platform.analytics.v1.TrackingEvent` with the corresponding `EventType`, wraps it in a
`platform.events.v1.EventEnvelope` (`type = "platform.analytics.v1.TrackingEvent"`), and publishes
it to the Kafka topic `analytics.events`. The endpoint SHALL remain pure edge telemetry —
validate, stamp identity, forward — holding no business logic and owning no analytics storage
(architecture Rule 2).

#### Scenario: A browsing beacon becomes a tracking event on analytics.events

- **WHEN** the gateway receives a valid `POST /api/track` beacon for a product view
- **THEN** it publishes exactly one `EventEnvelope` to the `analytics.events` topic whose `type` is
  `platform.analytics.v1.TrackingEvent` and whose payload unmarshals to a `TrackingEvent` with
  `EventType = EVENT_TYPE_VIEW` and the beacon's behavioral context (listing id, session id,
  page/path)

#### Scenario: A malformed beacon is rejected without producing

- **WHEN** the gateway receives a beacon with no recognizable event type or a malformed body
- **THEN** it responds with a client error and publishes nothing to `analytics.events`

### Requirement: Tracking events carry the resolved principal, never PII in the payload

The gateway SHALL set the `EventEnvelope.principal` from the edge-resolved principal (the same
`x-principal-*` identity resolved for every request; anonymous callers yield the anonymous
principal) and SHALL NOT copy authenticated user identity into `TrackingEvent` fields. The beacon
body SHALL carry only behavioral context (listing id, session id, anonymous id, page/path,
referrer, result position, query, and a `properties` map).

#### Scenario: Authenticated browsing attributes the event via the envelope

- **WHEN** a logged-in buyer's beacon is collected at the gateway
- **THEN** the produced `EventEnvelope.principal` identifies the buyer while the `TrackingEvent`
  payload contains no authenticated user id in its own fields

### Requirement: Telemetry emission is best-effort and does not block browsing

The frontend SHALL fire the tracking beacon asynchronously (`navigator.sendBeacon`, or a
`keepalive` fetch fallback) so that recording an event never blocks or fails the user's browsing
action, and the gateway producer SHALL degrade to a no-op when Kafka is disabled
(`KAFKA_ENABLED=false`) without erroring the request.

#### Scenario: A browsing action still succeeds when the event cannot be delivered

- **WHEN** the buyer performs a tracked action (view, click, or add-to-cart) while the analytics
  producer is disabled or the broker is unavailable
- **THEN** the browsing action completes normally and no user-visible error is shown

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
