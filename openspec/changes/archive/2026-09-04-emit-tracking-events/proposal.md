## Why

`add-tracking-event-contract` (archived) defined `platform.analytics.v1.TrackingEvent` +
`EventType` in `platform-core/packages/proto/platform/analytics/v1/analytics.proto`, carried on
the shared `platform.events.v1.EventEnvelope` and the dedicated Kafka topic `analytics.events`.
The contract exists but nothing produces to it: no surface in the platform records browsing
behavior. This change makes the platform emit real tracking events — a browsing action in the
browser (view / click / add-to-cart / search impression) becomes a `TrackingEvent` on
`analytics.events` — so the later warehouse sink (`build-warehouse-writer`) and recommendation
engine have a live stream to consume. It reuses the already-generated contract (no proto change)
and the proven franz-go `EventEnvelope` publish pattern from `team-domain`.

## What Changes

- **team-gateway** — add an **edge telemetry collector**: a lightweight HTTP endpoint
  (`POST /api/track`) that accepts a browser beacon (a single event, or a small batch), maps it to
  a `platform.analytics.v1.TrackingEvent`, wraps it in a `platform.events.v1.EventEnvelope`
  (`type = "platform.analytics.v1.TrackingEvent"`, `principal` = the edge-resolved principal,
  `request_id`), and publishes it to the Kafka topic `analytics.events`. The gateway today has **no
  Kafka producer**; this adds a minimal franz-go producer (envelope + produce only — no business
  logic), mirroring `team-domain/internal/events/publisher.go`. New config
  (`KAFKA_ENABLED`, `KAFKA_BROKERS`, `KAFKA_ANALYTICS_TOPIC`) wired through
  `internal/config/config.go` + `docker-compose.services.yaml`. The producer degrades to a no-op
  when `KAFKA_ENABLED=false`.
- **team-frontend** — add a small browser client util (`src/lib/track.ts`) that fires a lightweight
  beacon (`navigator.sendBeacon`, `fetch(..., { keepalive: true })` fallback) to the gateway
  collector on view / click / add-to-cart / search-result impression, and call it from the
  existing pages/components that already handle those actions (listing card + PDP view/click, cart
  add, search results). The beacon carries only behavioral context (listing id, session id,
  anonymous id, page/path, referrer, result position, query) — **no PII**; identity is attached at
  the edge via the envelope `principal`.

### Architecture note (Rule 2 tension, called out explicitly)

Rule 2 says the gateway holds **no business logic**. A tracking collector is compatible only when
it stays **pure edge telemetry**: validate the beacon shape, stamp the resolved principal, and
forward to Kafka — the gateway makes **no decision about the event's meaning**, owns no analytics
DB, and does no aggregation/enrichment. All interpretation happens downstream in the warehouse
consumer. Framed this way the collector is the same class of edge concern as auth resolution and
rate limiting (which already live at the gateway), not a business capability. This change deliberately
keeps the collector to `validate → wrap → produce` for that reason.

## Non-goals

- The warehouse **sink/consumer** of `analytics.events` — that is `build-warehouse-writer` (next change).
- Dashboards, reporting, or any read/aggregation over the events.
- Any **proto change** — `platform.analytics.v1` already exists and is vendored; this change adds
  no message/RPC and does not touch platform-core.
- **PII in the payload** — authenticated user identity travels only in `envelope.principal`, never
  in `TrackingEvent` fields (contract already forbids it).
- A transactional outbox / delivery guarantees for beacons — telemetry is best-effort
  (fire-and-forget); a dropped beacon is acceptable and logged, not surfaced to the user.
