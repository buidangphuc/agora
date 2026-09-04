## Why

The platform has no analytics/tracking contract. Behavioral events (product views, clicks,
add-to-cart, search-result impressions) are 500–1000× the volume of orders and must not land in
the transactional Postgres DBs; they belong on a dedicated stream feeding a data warehouse
(DuckDB local / BigQuery prod) and, later, the recommendation engine. Before any producer or
consumer can be built, the **event shape and topic must be defined once in the contract**
(architecture rule 4: a message is defined only in `platform-core/packages/proto`).

This change adds that contract and nothing else. It is the root dependency for `emit-tracking-events`
(gateway/frontend producers), `build-warehouse-writer` (the sink), and the P2 recommendation line.

The existing `platform.events.v1.EventEnvelope` (`platform/events/v1/events.proto`) already carries
the cross-cutting context (who/when/trace/request-id) and an opaque `payload` switched on a
fully-qualified `type` — exactly how `platform.listing.v1.ListingChanged` rides `listing.events`
today. Tracking events reuse that envelope; only the domain payload message and the topic name are new.

## What Changes

- **platform-core (`packages/proto`)** — add a new file `platform/analytics/v1/analytics.proto`
  (package `platform.analytics.v1`) defining a single `TrackingEvent` message with an
  `EventType` enum (`EVENT_TYPE_UNSPECIFIED`, `VIEW`, `CLICK`, `ADD_TO_CART`, `IMPRESSION`),
  the behavioral fields (listing id, session id, anonymous id, page/path, referrer, position,
  search query) and a `map<string,string> properties` escape hatch. Consumers switch on
  `type == "platform.analytics.v1.TrackingEvent"` and unmarshal it from `EventEnvelope.payload`.
- **platform-core** — standardize the Kafka topic name **`analytics.events`** (documented alongside
  `listing.events` in the data/infra docs; the constant itself lands in producers/consumers in later
  changes, not here).
- **Verification** — `buf lint` and `buf breaking` clean (additive, new package only). No generated
  code committed (each consumer re-vendors + `buf generate` in-tree per ADR-0001).

## Non-goals

- **No producer/consumer code.** No gateway collector, no frontend beacon, no warehouse writer —
  those are `emit-tracking-events` and `build-warehouse-writer`.
- **No re-vendor / regenerate** in any `team-*` repo in this change (the contract is added; consumers
  pull it when they implement).
- **No warehouse, DuckDB/BigQuery, Spark, or recommendation** logic.
- **No user-facing behavior** — this is a contract-only change, so it has no `FEATURES.yaml`/e2e track
  (see tasks.md; verification is `buf` + review, per the E2E-contract carve-out for non-user-facing changes).
- **No PII in the payload** by design — user identity travels in `EventEnvelope.principal`; the
  payload carries `anonymous_id`/`session_id` only.
