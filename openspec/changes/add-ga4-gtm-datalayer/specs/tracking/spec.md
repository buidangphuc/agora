# Specification: GA4 / GTM Data Layer Pattern

## Requirements

### Requirement: Client Data Layer and Event Dispatcher
The frontend MUST maintain a `window.dataLayer` event queue conforming to GA4 Enhanced Ecommerce standards.
- Before every ecommerce event push, the dispatcher MUST execute `window.dataLayer.push({ ecommerce: null })` to prevent parameter contamination across events.
- Events MUST carry an `ecommerce` payload with `currency`, `value`, `coupon`, and an array of `items`.
- Telemetry dispatch MUST be non-blocking, safe during SSR, and MUST NOT throw exceptions into UI components.

### Requirement: Flat Item Fan-out with Correlation
When multi-item events occur (e.g. `view_item_list`, `begin_checkout`, `purchase`), the client dispatcher MUST fan out the $N$ items into $N$ individual beacon records sent to `/api/track`.
- All records originating from the same multi-item event MUST share a single unique `event_group_id`.
- Each record MUST carry its item-level `price`, `quantity`, `item_category`, `item_list_id`, and `position` (`index`).

### Requirement: Client-Side Batch Queue
The frontend MUST batch individual beacon records into an array and transmit them in a single HTTP POST request to `/api/track`.
- The queue MUST flush automatically when:
  1. Queue length reaches $\ge 20$ records.
  2. Idle time exceeds $2$ seconds.
  3. The page enters `visibilitychange: hidden` or `beforeunload`.

### Requirement: Edge Collector Ingestion and Alias Mapping
The edge gateway `/api/track` endpoint MUST accept single objects or arrays of beacons up to 100 items.
- The collector MUST support both internal canonical names (`impression`, `click`, `view`, etc.) and GA4 aliases (`view_item_list`, `select_item`, `view_item`, etc.).
- The collector MUST perform atomic validation and reject batches exceeding limits with appropriate HTTP status codes without producing partial messages.

### Requirement: Idempotent Warehouse Evolution and GA4 Read View
The analytics warehouse (DuckDB and BigQuery) MUST apply idempotent schema migrations on startup adding new columns if not present.
- The warehouse MUST provide a `ga4_events` read-model view projecting canonical columns to GA4 standard names.
- The seller funnel query MUST project `begin_checkouts` and `purchases` alongside impressions, views, adds, and orders.
