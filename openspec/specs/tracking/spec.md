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
it to the Kafka topic `analytics.events`. Each event of a batch is validated on its own (see
`tracking-ingest-integrity`): invalid events are dropped, and only a body with no valid event is refused. The endpoint SHALL remain pure edge telemetry —
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

### Requirement: Client data layer
The frontend SHALL push GA4 ecommerce events to `window.dataLayer`, SHALL push `{ ecommerce: null }` before every ecommerce push, and SHALL NOT throw into UI components or during SSR.

#### Scenario: dataLayer receives ecommerce items
- **WHEN** a product list of 3 items renders
- **THEN** `window.dataLayer` contains a `view_item_list` entry whose `ecommerce.items` has length 3 and is immediately preceded by `{ ecommerce: null }`

#### Scenario: Dispatch during SSR is a no-op
- **WHEN** the dispatcher is called while `window` is undefined
- **THEN** the call returns without throwing and no beacon is queued
- **VERIFIED BY**: `team-frontend/src/lib/analytics/fanout.test.ts` "[Dispatch during SSR is a no-op] no window: returns without throwing and queues nothing"; `team-frontend/src/lib/analytics/dataLayer.test.ts` "[Dispatch during SSR is a no-op] pushDataLayer without window returns without throwing"

#### Scenario: Failing transport does not surface an error
- **WHEN** the `/api/track` fetch rejects
- **THEN** the queue retries once with the same body and no exception reaches the calling component
- **VERIFIED BY**: `team-frontend/src/lib/analytics/fanout.test.ts` "[Failing transport does not surface an error] rejected fetch: no throw into caller, one retry with same body"

### Requirement: Conditional GTM loading
The frontend SHALL load the GTM script only when `NEXT_PUBLIC_GTM_ID` is non-empty.

#### Scenario: No GTM when ID is empty
- **WHEN** a page loads with `NEXT_PUBLIC_GTM_ID` empty
- **THEN** the DOM has no `googletagmanager.com` script and the browser issues 0 requests to `googletagmanager.com`
- **VERIFIED BY**: `team-frontend/src/lib/analytics/destinations/gtm.test.ts` "[No GTM when ID is empty] NEXT_PUBLIC_GTM_ID unset adds no script"

#### Scenario: GTM loads when ID is set
- **WHEN** a page loads with `NEXT_PUBLIC_GTM_ID=GTM-TEST`
- **THEN** exactly 1 script element with `id=GTM-TEST` in its `src` is present

#### Scenario: No external request to Google in e2e
- **WHEN** a page loads in the e2e stack with `NEXT_PUBLIC_GTM_ID=GTM-TEST`
- **THEN** the GTM script is served by the mock (stub response to the intercepted `googletagmanager.com` / `google-analytics.com` route) and 0 requests leave the stack to Google hosts

### Requirement: Data layer privacy
The frontend SHALL push to `window.dataLayer` only allowlisted keys, SHALL nest caller-supplied `properties` under a single `properties` key (never spread at top level, never overriding the reserved keys `event` and `ecommerce`), and SHALL scrub `search_term`, `page_path` and `page_referrer` the same way the gateway does (emails, phone numbers, digit runs of 13+; referrer reduced to `scheme://host/path`).

#### Scenario: Caller properties cannot override reserved keys or leak to top level
- **WHEN** a dispatch passes `properties` of `{ event: "purchase", ecommerce: { value: 1 }, email: "a@b.vn", note: "x" }`
- **THEN** the pushed entry's top-level `event` and `ecommerce` are the dispatcher's own values, the top level has no `email` or `note` key, and only allowlisted keys appear under the single `properties` key
- **VERIFIED BY**: `team-frontend/src/lib/analytics/dataLayer.privacy.test.ts` "[Caller properties cannot override reserved keys or leak to top level]"

#### Scenario: Search term and path are scrubbed in the dataLayer
- **WHEN** a search is dispatched with `search_term` "iphone lan.nguyen@gmail.com 0912345678 1234567890123" and `page_path` "/search?q=0912345678"
- **THEN** the dataLayer entry holds `search_term` "iphone [email] [phone] [number]" and `page_path` "/search?q=[phone]", and no raw email or digit run is present
- **VERIFIED BY**: `team-frontend/src/lib/analytics/dataLayer.privacy.test.ts` "[Search term and path are scrubbed in the dataLayer]"

#### Scenario: Referrer is reduced to scheme host path
- **WHEN** a page view is dispatched with `page_referrer` "https://shop.example.vn/p/12?email=a@b.vn#x"
- **THEN** the dataLayer entry holds `page_referrer` "https://shop.example.vn/p/12"; a referrer of "not a url" becomes ""
- **VERIFIED BY**: `team-frontend/src/lib/analytics/dataLayer.privacy.test.ts` "[Referrer is reduced to scheme host path]"

### Requirement: Batched beacon queue and item fan-out
The frontend SHALL fan a multi-item event out into one beacon per item sharing one `event_group_id`, and SHALL send queued beacons as one array in a single POST to `/api/track`, flushing at 20 events, 2 s idle, or `visibilitychange: hidden`.

#### Scenario: One request per rendered list
- **WHEN** a grid of 12 items renders and 2 s elapse
- **THEN** the browser sends exactly 1 `POST /api/track` whose body is an array of 12 beacons

#### Scenario: Size flush boundary
- **WHEN** the 20th beacon is enqueued
- **THEN** the queue flushes immediately in one request carrying 20 beacons
- **VERIFIED BY**: `team-frontend/src/lib/analytics/fanout.test.ts` "[Size flush boundary] 19 beacons stay queued; the 20th flushes immediately as one request of 20"

#### Scenario: Page hidden flushes pending beacons
- **WHEN** `visibilitychange` fires with state hidden while 3 beacons are queued
- **THEN** the queue sends those 3 beacons before the page is suspended
- **VERIFIED BY**: `team-frontend/src/lib/analytics/fanout.test.ts` "[Page hidden flushes pending beacons] visibilitychange hidden with 3 queued sends all 3 before any timer"

#### Scenario: Fan-out rows share a group id
- **WHEN** a `view_item_list` of N=5 items is dispatched
- **THEN** 5 beacons are produced, each with a distinct `event_id`, the same non-empty `event_group_id`, and its own `position`, `price`, `item_list_id`

### Requirement: Edge collector aliases and batch cap
The gateway SHALL accept a single beacon or an array of at most 100 beacons, SHALL map the GA4 aliases `view_item` (view), `select_item` (click) and `view_item_list` (impression) to canonical `EventType`, and SHALL reject an oversized batch atomically.

#### Scenario: GA4 alias accepted
- **WHEN** a beacon with type `view_item_list` and ecommerce fields is posted
- **THEN** the gateway publishes a `TrackingEvent` with the canonical impression type and the same `item_list_id`, `currency`, `value`
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` TestHandleTrackEcommerceBatchAndGA4Aliases

#### Scenario: Batch of 101 rejected
- **WHEN** an array of 101 beacons is posted to `/api/track`
- **THEN** the gateway responds 400 and publishes 0 messages
- **VERIFIED BY**: `team-gateway/internal/edge/collector_test.go` TestHandleTrackExceedsBatchLimit

#### Scenario: Batch of exactly 100 accepted
- **WHEN** an array of 100 valid beacons is posted
- **THEN** the gateway responds 2xx and publishes 100 messages
- **VERIFIED BY**: `team-gateway/internal/edge/collector_boundary_test.go` TestBoundaryExactly100BatchAccepted

#### Scenario: Malformed event does not drop its siblings
- **WHEN** a batch of 10 contains 1 beacon with an unknown event type
- **THEN** the gateway publishes 9 messages and drops the invalid one
- **VERIFIED BY**: `team-gateway/internal/edge/collector_integrity_test.go` TestTrackMixedBatchDropsOnlyInvalid

#### Scenario: Anonymous caller allowed
- **WHEN** a request without a bearer token posts a valid beacon
- **THEN** the gateway publishes the event with a principal of type PRINCIPAL_TYPE_ANONYMOUS (no authenticated identity) and responds 2xx
- **VERIFIED BY**: `team-gateway/internal/edge/collector_boundary_test.go` TestBoundaryAnonymousCallerAllowed

### Requirement: Edge handler latency
The `/api/track` handler SHALL process a 100-event batch with p99 latency below 50 ms, excluding the broker path.

#### Scenario: Handler p99 under budget
- **WHEN** a Go benchmark posts 1000 requests of 100 valid events each to the handler with a fake publisher
- **THEN** the measured p99 latency is below 50 ms
- **VERIFIED BY**: `team-gateway/internal/edge/collector_boundary_test.go` TestBoundaryHandlerP99UnderBudget

#### Scenario: Oversized body rejected without publishing
- **WHEN** a request body larger than 64 KiB is posted
- **THEN** the handler responds with a 4xx status and publishes 0 messages
- **VERIFIED BY**: `team-gateway/internal/edge/collector_boundary_test.go` TestBoundaryOversizedBodyRejectedWithoutPublishing

### Requirement: Idempotent warehouse evolution and GA4 view
The warehouse adapters (DuckDB and BigQuery) SHALL add the new tracking columns with `ADD COLUMN IF NOT EXISTS` on startup and SHALL expose a `ga4_events` view with identical column names in both engines.

#### Scenario: DuckDB migrates a legacy database
- **WHEN** the worker opens a database whose `tracking_events` lacks the new columns, twice in a row
- **THEN** both opens succeed, every new column exists once, and prior rows are preserved with NULL in the new columns
- **VERIFIED BY**: `team-analytics/internal/warehouse/duckdb/duckdb_test.go` TestOpenMigratesLegacyTrackingColumns_DuckDBMigratesALegacyDatabase

#### Scenario: BigQuery migration is idempotent
- **WHEN** the BigQuery adapter runs its startup migration on a table that already has the new columns
- **THEN** the migration issues `ADD COLUMN IF NOT EXISTS` statements, returns no error, and changes no schema
- **VERIFIED BY**: `team-analytics/internal/warehouse/bigquery/migrate_test.go` TestBigQueryMigrationIsIdempotent_IssuesAddColumnIfNotExists

#### Scenario: BigQuery migration failure is surfaced
- **WHEN** the BigQuery `ALTER TABLE` call returns an error
- **THEN** startup fails with an error naming the table and no ingest begins
- **VERIFIED BY**: `team-analytics/internal/warehouse/bigquery/migrate_test.go` TestBigQueryMigrationFailureIsSurfaced_NamesTable

#### Scenario: Schema parity
- **WHEN** `warehouse.Schema` is compared with the DuckDB and BigQuery column lists
- **THEN** the three lists have the same names in the same order, and the `ga4_events` view exposes the same projected column names in both engines
- **VERIFIED BY**: `team-analytics/internal/warehouse/bigquery/migrate_test.go` TestSchemaParity_BigQueryColumnsMatchWarehouseSchema and TestSchemaParity_GA4EventsViewColumnsMatchDuckDB; `team-analytics/internal/warehouse/warehouse_test.go` TestSchemaParity

#### Scenario: Funnel purchases count distinct transaction ids
- **WHEN** `GetSellerFunnel` runs for a seller with 3 `purchase` events sharing `transaction_id=o-1` and 1 `purchase` with `transaction_id=o-2`
- **THEN** the response has `purchases=2`
- **VERIFIED BY**: `team-analytics/internal/query/service_test.go` TestGetSellerFunnel_CheckoutPurchase (case "Funnel purchases count distinct transaction ids")

#### Scenario: Funnel projects checkout and purchase
- **WHEN** `GetSellerFunnel` runs for a seller with 2 `begin_checkout` and 1 `purchase` events
- **THEN** the response has `begin_checkouts=2` and `purchases=1`
- **VERIFIED BY**: `team-analytics/internal/query/service_test.go` TestGetSellerFunnel_CheckoutPurchase (case "Funnel projects checkout and purchase")

### Requirement: Ecommerce columns in Parquet export
The export job SHALL write `item_list_id`, `position`, `event_group_id`, `transaction_id`, `currency`, and `value` to the tracking Parquet file.

#### Scenario: Exported file has ecommerce columns
- **WHEN** an export cycle completes after a purchase event with `transaction_id=o-1`, `currency=VND`, `value=500000` is stored
- **THEN** the Parquet file has the 6 columns and the row holds those values
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` TestExport_ExportedFileHasEcommerceColumns

#### Scenario: Failed export keeps previous file
- **WHEN** the export query fails
- **THEN** the previous Parquet file remains unchanged and readable
- **VERIFIED BY**: `team-analytics/internal/export/export_test.go` TestExport_FailedExportKeepsPreviousFile

### Requirement: Tolerant consumers and purchase dedupe
Featurestore and recsys SHALL read the ecommerce columns only when present, SHALL read legacy Parquet without them, and SHALL count at most one purchase interaction per (buyer, listing, `transaction_id`); a multi-item order counts each listing once.

#### Scenario: Legacy Parquet without new columns
- **WHEN** featurestore or recsys loads a Parquet file lacking the ecommerce columns
- **THEN** loading succeeds and features equal those computed before this change
- **VERIFIED BY**: `platform-featurestore/tests/test_dataset.py` test_legacy_parquet_without_new_columns_loads_and_features_unchanged; `platform-recsys/tests/test_dataset.py` test_legacy_dataset_without_new_columns_resolves; `platform-recsys/tests/test_interactions_spark.py` test_dataset_triples_legacy_columns_only_is_unchanged

#### Scenario: New columns consumed when present
- **WHEN** the Parquet file contains `item_list_id` and `position`
- **THEN** the loaded frame exposes both columns to feature code
- **VERIFIED BY**: `platform-featurestore/tests/test_dataset.py` test_new_columns_consumed_when_present; `platform-recsys/tests/test_interactions_spark.py` test_dataset_triples_tolerates_new_columns_present

#### Scenario: Duplicate purchase counted once
- **WHEN** 3 `purchase` rows share `transaction_id=o-1` for one buyer and one listing
- **THEN** the buyer has 1 purchase interaction for that listing
- **VERIFIED BY**: `platform-featurestore/tests/test_dataset.py` test_duplicate_purchase_counted_once

#### Scenario: Purchase without transaction id is not merged
- **WHEN** 2 `purchase` rows have an empty `transaction_id`
- **THEN** both rows are kept and no row is dropped by dedupe
- **VERIFIED BY**: `platform-featurestore/tests/test_dataset.py` test_purchase_without_transaction_id_is_not_merged

### Requirement: Revenue source of truth
The system SHALL NOT derive revenue from `tracking_events`; `order_facts` SHALL remain the sole revenue source.

#### Scenario: Tracking value excluded from revenue
- **WHEN** a `purchase` tracking event carries `value=999999` and the order's `order_facts` total is 500000
- **THEN** every revenue query reports 500000 for that order
- **VERIFIED BY**: `team-analytics/internal/query/revenue_guard_test.go` TestRevenueGuard (subtests "Tracking value excluded from revenue/*")

#### Scenario: Tracking purchase without order not counted as revenue
- **WHEN** a `purchase` tracking event exists for a `transaction_id` with no `order_facts` row
- **THEN** revenue for that transaction is 0
- **VERIFIED BY**: `team-analytics/internal/query/revenue_guard_test.go` TestRevenueGuard (subtest "Tracking purchase without order not counted as revenue")

### Requirement: No personal data in telemetry
Telemetry payloads SHALL NOT contain email, phone, name, or address.

#### Scenario: Payload keys limited to schema
- **WHEN** a purchase dispatch is captured from `dataLayer` and the beacon body
- **THEN** neither contains a key or value matching email, phone, or name of the buyer

#### Scenario: Oversized properties rejected
- **WHEN** a beacon has more than 20 property keys
- **THEN** the gateway drops that event and does not publish it
- **VERIFIED BY**: `team-gateway/internal/edge/collector_integrity_test.go` TestTrackBounds (cases "20 props" accepted, "21 props" dropped)
