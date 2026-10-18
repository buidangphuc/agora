## Why

The initiative `q4-loop-instrumentation` (locked 2026-10-15) needs activation, loop and AI-interaction actions to reach `analytics.events`. The edge collector (`HandleTrack`, `team-gateway/internal/edge/collector.go`) only accepts the beacon types in `beaconEventTypes`; any other `type` is dropped as "unknown event type". Without this slice the frontend slices (`frontend-loop-events`, `frontend-ai-interaction-events`) have nowhere to send the seven new beacons. The collector is also the only place where free text can be kept out of the warehouse for AI events, so it enforces the `surface`/`action` allowlists.

initiative: q4-loop-instrumentation

## What Changes

- **team-gateway** (`internal/edge/collector.go`, `internal/edge/collector_test.go`)
  - `beaconEventTypes` gains seven entries mapping the lowercase beacon name to the new enum value from `q4-loop-events-contract`: `sign_up` -> `EVENT_TYPE_SIGN_UP`, `login` -> `EVENT_TYPE_LOGIN`, `follow_seller` -> `EVENT_TYPE_FOLLOW_SELLER`, `referral_redeem` -> `EVENT_TYPE_REFERRAL_REDEEM`, `daily_check_in` -> `EVENT_TYPE_DAILY_CHECK_IN`, `alert_subscribe` -> `EVENT_TYPE_ALERT_SUBSCRIBE`, `ai_interaction` -> `EVENT_TYPE_AI_INTERACTION`. Existing entries and GA4 aliases are untouched.
  - `validateBeacon` adds one rule for `ai_interaction`: `properties["surface"]` must be exactly one of `assistant`, `chat_copilot`, `magic_listing`, `review_summary` and `properties["action"]` exactly one of `shown`, `accepted`, `edited`, `dismissed` (case-sensitive, no trimming). `properties` must contain no other key. Otherwise the beacon is invalid, which the existing flow already turns into "dropped"; a body with no valid beacon answers 400 and publishes nothing.
  - The other six new types use the existing generic bounds only (id length, 20 property keys, key 40 / value 256 chars, free-text scrubbing). `HandleTrack` flow, the `WireTrackBeacon` format, principal resolution from the `session` cookie, consent handling and the 202 `{accepted, dropped}` body are unchanged.
- **platform-e2e**: new feature `tests/e2e/features/tracking/loop_event_intake.feature` with bindings `tests/e2e/step_definitions/test_loop_event_intake.py` (reuses `tracking_steps.py` helpers; adds no helper under `support/`).
- Not changed: proto (owned by `q4-loop-events-contract`; this slice consumes the regenerated `EventType` values), `ga4relay`, `ga4import`, `ga4_reconcile.go`, team-analytics, the frontend.

## Impact

- Affected specs: new capability `edge-tracking-collector` (this change).
- Backward compatible: existing beacon types behave as before; the only new rejection applies to the new `ai_interaction` type.
- Depends on `q4-loop-events-contract`: run `make proto` in team-gateway (the vendored `platform/analytics/v1` copy and `generated/`) before coding.
- Deploy ordering: gateway before the frontend slices. Until team-analytics ships `analytics-loop-event-storage`, the warehouse stores the new enums as `unspecified`; that is tolerated (no data loss on the topic).

## NFR

- Latency: collector added latency < 1 ms per beacon (p99, excluding network and Kafka produce). The added work is a map lookup plus two set membership checks; collector p95 stays within the existing budget (no new network call, no new allocation beyond the existing property copy).
- Availability: unchanged; delivery stays best-effort (a produce error is logged and the beacon still counts as accepted).
- Limits: unchanged (`maxBeaconBytes`, `maxBatchItems`, property bounds above). The AI allowlist is a fixed in-memory set, not configurable.
- Compatibility: `buf breaking` safe (additive enum values; consumed only).

## Data classification

No new personal data is collected or stored by this slice.
- Envelope principal id (existing; resolved from the `session` cookie, never from the beacon body): purpose = attribute activation and loop actions to a user for funnel analysis; consent basis = unchanged from existing first-party tracking (consent snapshot attached to each event); retention = unchanged; same as other analytics.events rows (no explicit retention policy found in the repo docs).
- `properties.surface`, `properties.action` (new, AI events): closed-vocabulary enum strings, not personal data; purpose = measure AI usage; retention as above. Anything outside the allowlist is rejected, so no prompt, reply, title or review text can enter through these keys.
- `properties.method`, `seller_id`, `alert_type`, `streak`, `listingId` on the other new types: business identifiers or closed values; still pass through the existing text scrubber (emails, phone numbers, 13+ digit runs).
- Referral codes are never carried (the frontend sends no property; the collector has no code field).

## Threat model

- Assets: integrity of the warehouse event vocabulary; absence of free text and PII in `analytics.events`.
- Abuse cases:
  - A client stuffs prompt or reply text into `ai_interaction` `surface`/`action` to exfiltrate or poison data. Mitigation: exact allowlist match; failure drops the beacon (400 when alone).
  - Event inflation or forged funnel events (fake `sign_up`). The beacon endpoint is unauthenticated by design (best-effort telemetry), so these counts are advisory, and the warehouse is not a billing or authorisation source. Mitigations: per-event bounds, batch cap, existing request limits; the principal on the envelope comes from the verified `session` cookie, a forged body cannot set it.
  - Case or whitespace variants (`Assistant`, ` shown`) slipping past the allowlist. Mitigation: exact comparison, no normalisation of property values.
  - Free text smuggled under an extra property key of `ai_interaction`. Mitigation: only the keys `surface` and `action` are allowed on that type (human decision, design.md).
  - Oversized or numerous properties. Mitigation: existing generic bounds apply to the new types.
- Authz rule: `/api/track` stays open to anonymous callers (as today); no permission is added or relaxed. An anonymous caller is attributed to the anonymous principal; an authenticated one to the cookie's principal.

## Open items

- The six non-AI new types are not value-validated beyond the generic bounds (per the initiative spec); a forged `sign_up` from any client is accepted. Accepted risk, see threat model.
