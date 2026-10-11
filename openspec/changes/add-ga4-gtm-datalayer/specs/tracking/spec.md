# Specification: GA4 / GTM Data Layer Pattern

## ADDED Requirements

### Requirement: Client data layer
The frontend SHALL push GA4 ecommerce events to `window.dataLayer`, SHALL push `{ ecommerce: null }` before every ecommerce push, and SHALL NOT throw into UI components or during SSR.

#### Scenario: dataLayer receives ecommerce items
- **WHEN** a product list of 3 items renders
- **THEN** `window.dataLayer` contains a `view_item_list` entry whose `ecommerce.items` has length 3 and is immediately preceded by `{ ecommerce: null }`

#### Scenario: Dispatch during SSR is a no-op
- **WHEN** the dispatcher is called while `window` is undefined
- **THEN** the call returns without throwing and no beacon is queued

#### Scenario: Failing transport does not surface an error
- **WHEN** the `/api/track` fetch rejects
- **THEN** the queue retries once with the same body and no exception reaches the calling component

### Requirement: Conditional GTM loading
The frontend SHALL load the GTM script only when `NEXT_PUBLIC_GTM_ID` is non-empty.

#### Scenario: No GTM when ID is empty
- **WHEN** a page loads with `NEXT_PUBLIC_GTM_ID` empty
- **THEN** the DOM has no `googletagmanager.com` script and the browser issues 0 requests to `googletagmanager.com`

#### Scenario: GTM loads when ID is set
- **WHEN** a page loads with `NEXT_PUBLIC_GTM_ID=GTM-TEST`
- **THEN** exactly 1 script element with `id=GTM-TEST` in its `src` is present

### Requirement: Batched beacon queue and item fan-out
The frontend SHALL fan a multi-item event out into one beacon per item sharing one `event_group_id`, and SHALL send queued beacons as one array in a single POST to `/api/track`, flushing at 20 events, 2 s idle, or `visibilitychange: hidden`.

#### Scenario: One request per rendered list
- **WHEN** a grid of 12 items renders and 2 s elapse
- **THEN** the browser sends exactly 1 `POST /api/track` whose body is an array of 12 beacons

#### Scenario: Size flush boundary
- **WHEN** the 20th beacon is enqueued
- **THEN** the queue flushes immediately in one request carrying 20 beacons

#### Scenario: Page hidden flushes pending beacons
- **WHEN** `visibilitychange` fires with state hidden while 3 beacons are queued
- **THEN** the queue sends those 3 beacons before the page is suspended

#### Scenario: Fan-out rows share a group id
- **WHEN** a `view_item_list` of N=5 items is dispatched
- **THEN** 5 beacons are produced, each with a distinct `event_id`, the same non-empty `event_group_id`, and its own `position`, `price`, `item_list_id`

### Requirement: Edge collector aliases and batch cap
The gateway SHALL accept a single beacon or an array of at most 100 beacons, SHALL map the GA4 aliases `view_item` (view), `select_item` (click) and `view_item_list` (impression) to canonical `EventType`, and SHALL reject an oversized batch atomically.

#### Scenario: GA4 alias accepted
- **WHEN** a beacon with type `view_item_list` and ecommerce fields is posted
- **THEN** the gateway publishes a `TrackingEvent` with the canonical impression type and the same `item_list_id`, `currency`, `value`

#### Scenario: Batch of 101 rejected
- **WHEN** an array of 101 beacons is posted to `/api/track`
- **THEN** the gateway responds 400 and publishes 0 messages

#### Scenario: Batch of exactly 100 accepted
- **WHEN** an array of 100 valid beacons is posted
- **THEN** the gateway responds 2xx and publishes 100 messages

#### Scenario: Malformed event does not drop its siblings
- **WHEN** a batch of 10 contains 1 beacon with an unknown event type
- **THEN** the gateway publishes 9 messages and drops the invalid one

#### Scenario: Anonymous caller allowed
- **WHEN** a request without a bearer token posts a valid beacon
- **THEN** the gateway publishes the event with an empty principal and responds 2xx

### Requirement: Edge handler latency
The `/api/track` handler SHALL process a 100-event batch with p99 latency below 50 ms, excluding the broker path.

#### Scenario: Handler p99 under budget
- **WHEN** a Go benchmark posts 1000 requests of 100 valid events each to the handler with a fake publisher
- **THEN** the measured p99 latency is below 50 ms

#### Scenario: Oversized body rejected without publishing
- **WHEN** a request body larger than 64 KiB is posted
- **THEN** the handler responds with a 4xx status and publishes 0 messages

### Requirement: Idempotent warehouse evolution and GA4 view
The warehouse adapters (DuckDB and BigQuery) SHALL add the new tracking columns with `ADD COLUMN IF NOT EXISTS` on startup and SHALL expose a `ga4_events` view with identical column names in both engines.

#### Scenario: DuckDB migrates a legacy database
- **WHEN** the worker opens a database whose `tracking_events` lacks the new columns, twice in a row
- **THEN** both opens succeed, every new column exists once, and prior rows are preserved with NULL in the new columns

#### Scenario: BigQuery migration is idempotent
- **WHEN** the BigQuery adapter runs its startup migration on a table that already has the new columns
- **THEN** the migration issues `ADD COLUMN IF NOT EXISTS` statements, returns no error, and changes no schema

#### Scenario: BigQuery migration failure is surfaced
- **WHEN** the BigQuery `ALTER TABLE` call returns an error
- **THEN** startup fails with an error naming the table and no ingest begins

#### Scenario: Schema parity
- **WHEN** `warehouse.Schema` is compared with the DuckDB and BigQuery column lists
- **THEN** the three lists have the same names in the same order, and the `ga4_events` view exposes the same projected column names in both engines

#### Scenario: Funnel purchases count distinct transaction ids
- **WHEN** `GetSellerFunnel` runs for a seller with 3 `purchase` events sharing `transaction_id=o-1` and 1 `purchase` with `transaction_id=o-2`
- **THEN** the response has `purchases=2`

#### Scenario: Funnel projects checkout and purchase
- **WHEN** `GetSellerFunnel` runs for a seller with 2 `begin_checkout` and 1 `purchase` events
- **THEN** the response has `begin_checkouts=2` and `purchases=1`

### Requirement: Ecommerce columns in Parquet export
The export job SHALL write `item_list_id`, `position`, `event_group_id`, `transaction_id`, `currency`, and `value` to the tracking Parquet file.

#### Scenario: Exported file has ecommerce columns
- **WHEN** an export cycle completes after a purchase event with `transaction_id=o-1`, `currency=VND`, `value=500000` is stored
- **THEN** the Parquet file has the 6 columns and the row holds those values

#### Scenario: Failed export keeps previous file
- **WHEN** the export query fails
- **THEN** the previous Parquet file remains unchanged and readable

### Requirement: Tolerant consumers and purchase dedupe
Featurestore and recsys SHALL read the ecommerce columns only when present, SHALL read legacy Parquet without them, and SHALL count at most one `purchase` per `transaction_id`.

#### Scenario: Legacy Parquet without new columns
- **WHEN** featurestore or recsys loads a Parquet file lacking the ecommerce columns
- **THEN** loading succeeds and features equal those computed before this change

#### Scenario: New columns consumed when present
- **WHEN** the Parquet file contains `item_list_id` and `position`
- **THEN** the loaded frame exposes both columns to feature code

#### Scenario: Duplicate purchase counted once
- **WHEN** 3 `purchase` rows share `transaction_id=o-1` for one buyer
- **THEN** the buyer's purchase count is 1

#### Scenario: Purchase without transaction id is not merged
- **WHEN** 2 `purchase` rows have an empty `transaction_id`
- **THEN** both rows are kept and no row is dropped by dedupe

### Requirement: Revenue source of truth
The system SHALL NOT derive revenue from `tracking_events`; `order_facts` SHALL remain the sole revenue source.

#### Scenario: Tracking value excluded from revenue
- **WHEN** a `purchase` tracking event carries `value=999999` and the order's `order_facts` total is 500000
- **THEN** every revenue query reports 500000 for that order

#### Scenario: Tracking purchase without order not counted as revenue
- **WHEN** a `purchase` tracking event exists for a `transaction_id` with no `order_facts` row
- **THEN** revenue for that transaction is 0

### Requirement: No personal data in telemetry
Telemetry payloads SHALL NOT contain email, phone, name, or address.

#### Scenario: Payload keys limited to schema
- **WHEN** a purchase dispatch is captured from `dataLayer` and the beacon body
- **THEN** neither contains a key or value matching email, phone, or name of the buyer

#### Scenario: Oversized properties rejected
- **WHEN** a beacon has more than 20 property keys
- **THEN** the gateway drops that event and does not publish it
