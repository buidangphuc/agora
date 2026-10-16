## Why

Initiative `q4-loop-instrumentation` (locked 2026-10-15) makes activation, loop and AI-surface actions land in the warehouse so signup -> first purchase and AI usage -> conversion are answerable with SQL. The tracking consumer's `eventTypeName` (`team-analytics/internal/consumer/tracking.go`) only knows EventType values 1-14; every other enum value is stored as `unspecified`, which would make the seven new events (`EVENT_TYPE_SIGN_UP` .. `EVENT_TYPE_AI_INTERACTION`, values 15-21 added by `q4-loop-events-contract`) indistinguishable in the warehouse. This slice teaches the consumer the new names.

initiative: q4-loop-instrumentation

## What Changes

- **team-analytics** (`internal/consumer/tracking.go`): `eventTypeName` maps the seven new values to `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe`, `ai_interaction`. All other mapping (existing 14 names, `unspecified` default) is unchanged. `properties` already flows through `TrackingRecord.Properties` into the JSON column; no schema or migration change (`event_type` is a STRING column, DuckDB and BigQuery parity unaffected).
- **team-analytics README**: event-type vocabulary list gains the seven names.
- **platform-e2e**: new feature `tests/e2e/features/tracking/loop_event_storage.feature` with bindings `tests/e2e/step_definitions/test_loop_event_storage.py`. It builds each TrackingEvent payload in the step file (event_type, properties, listing_id, consent), wraps it with `ga4_relay_support.envelope` and sends it onto `analytics.events` with `ga4_relay_support.produce` and reads the DuckDB warehouse with `tii_support.warehouse_rows`; it does not depend on the gateway or the frontend.
- Scope notes: the warehouse schema is unchanged and the GA4 relay, import and reconcile paths are untouched (`ga4relay`, `ga4import`, `ga4_reconcile.go`).
- Not changed: proto (contract slice), gateway intake (`gateway-loop-event-intake`), frontend, `ga4relay`, `ga4import`, `ga4_reconcile.go`, warehouse schema, any RPC or report (non-goal; reports are Q16).

## Impact

- Affected specs: new capability `loop-event-storage` (this change). Additive; rows written before this change are not rewritten.
- Depends on `q4-loop-events-contract`: vendored proto copy in team-analytics must carry EventType 15-21 (run `make proto` in team-analytics before coding).
- Deploy order: ship this consumer before or together with the gateway slice. If the gateway emits the new types first, rows land as `unspecified` and are not backfilled (the Kafka offset has advanced); accepted, recorded in Open items.

## NFR

- Latency/throughput: adds no I/O per event (a switch-case lookup only); batch size (`BATCH_MAX_SIZE`, default 500) and flush interval (`BATCH_FLUSH_INTERVAL_SECONDS`, default 2) are unchanged, as is offset-after-write.
- Availability: unchanged. An unrecognised enum value never fails the consumer; it is stored as `unspecified` (at-least-once, no poison message). An undecodable payload is still a consumer error, as today.
- Limits: `properties` size is bounded upstream at the gateway; this slice adds no new limit and stores the map as received.

## Data classification

No new personal data field is introduced by this slice; it only changes the label of an existing column value.
- `event_type` (new values): purpose = product analytics funnel/cohort queries; consent basis = existing first-party tracking basis (unchanged); retention = warehouse policy (unchanged).
- `properties` of the new types (`method`, `seller_id`, `streak`, `alert_type`, `surface`, `action`): business/UX identifiers, no free text, no referral code (enforced at the gateway and frontend, not here). Same retention as other properties.
- `principal_id` (envelope) on `sign_up`/`login` rows links an activation to a user: personal data, already stored for all tracking rows; DPO erasure handling follows the existing tracking-row policy (human item).
- `ga_client_id`: still blanked when `consent.analytics` is false, for the new types as well.
- E2E data: synthetic principal and session ids prefixed `e2e-loop-`; no real user data.

## Threat model

- Assets: warehouse integrity (event vocabulary the funnel queries depend on); consumer availability.
- Abuse cases:
  - A forged or future enum value crashes the consumer or corrupts a known bucket. Mitigation: unknown values map to `unspecified`; never an error, never a known name. Scenario asserts it.
  - Producer injects an event carrying a client id without consent. Mitigation: the existing blanking rule applies to every type; scenario asserts it for a new type.
  - Replay inflating funnel counts. Mitigation: existing one-row-per-`event_id` sink dedupe; scenario asserts it for a new type.
- Authz rule: none at this layer (no RPC). Trust boundary is the Kafka topic: only the gateway collector and service producers write to `analytics.events`; unchanged.

## Open items

- Rows emitted before this consumer is deployed stay `unspecified` (no backfill).
- DPO sign-off on `sign_up`/`login` rows linked to principal id (shared with the initiative's pii routing).
