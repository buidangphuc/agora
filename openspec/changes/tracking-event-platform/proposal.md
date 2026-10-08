## Why

The marketplace exists to serve recommendations and AI, but the behavioural data underneath is too thin to learn
from or to evaluate anything (program change **C1** in `plans/ai-first/PROGRAM.md`). Verified in code (2026-10-01):

- `platform.analytics.v1.TrackingEvent` has 4 event types (`VIEW`, `CLICK`, `ADD_TO_CART`, `IMPRESSION`) and **no
  attribution**: nothing says which placement, which serving request or which model produced an impression, so
  impressions cannot be joined to clicks and no model change can be evaluated online (`plans/mlops` P1-T1).
- `team-frontend` fires one beacon per event (`src/lib/track.ts`), counts a search impression the moment a result
  is rendered (`SearchImpressions.tsx`, not when it is seen), instruments only search, PDP view, card click and
  add-to-cart, stores the anonymous id in `localStorage` (SSR cannot read it, so anonymous `Recommend` calls cannot
  carry it), and has no login stitching or opt-out.
- `team-gateway`'s `POST /api/track` (`internal/edge/collector.go`) maps 4 types, rejects a whole batch on one bad
  element, has no per-event schema checks, no batch/field caps beyond 64 KiB, no per-visitor rate limit, no consent
  handling, no sampling, and stamps `occurred_at` at publish time with no client-time or skew handling.
- No server-side fact reaches analytics: `team-order` and `team-engagement` have **no outbox and no topic**, and
  `team-analytics` does not consume `payment.events`. The seller funnel counts "orders" from a
  `properties.order_id` that nothing sets.
- `team-analytics` appends to one `tracking_events` table with no dedupe (at-least-once means duplicates), no
  late-event policy, no identity stitching, no retention or erasure, and no data-quality signal. The feature store
  (C2) and the recommender (C3) would inherit all of that silently.

## What Changes

- **platform-core (contract, cross-service, additive only)**:
  - `analytics.proto`: new `EventType` values `PAGE_VIEW`, `SEARCH`, `REMOVE_FROM_CART`, `CHECKOUT_START`, `SHARE`,
    `IDENTIFY`, `CONSENT_UPDATE`; new `TrackingEvent` fields: attribution (`placement_id`, `request_id`,
    `model_version`), `dwell_ms`, `result_count`, `filters`, `client_event_id`, `client_occurred_at`, `quantity`,
    `share_channel`, `sample_rate`, `page_type`; a privacy-ops service (`AnalyticsOpsService`: inspect, erase,
    data-quality report), admin-scoped; its three RPCs join the gateway's pinned admin-only procedure set
    (today `QueryAuditLog`, `ReviewKyc`, `ResolveDispute`; C3 later adds `GetPlacementMetrics`).
  - `events.proto`: `EventEnvelope.schema_version`.
  - `order.proto`: `OrderPlaced`, `OrderStatusChanged` (on new topic `order.events`).
  - `engagement.proto`: `FavoriteChanged`, `ReviewCreated`, `FollowChanged` (on new topic `engagement.events`).
  - `recommendation.proto` / `search.proto`: `RecommendRequest.placement_id = 6`; `RecommendResponse.request_id = 3`
    and `placement_id = 4`; `SearchListingsResponse.request_id = 4` and `model_version = 5`. **This change owns these
    serving attribution fields** (program resolution, `plans/ai-first/PROGRAM.md`); C3 (`recommendations-end-to-end`)
    and the search owners only populate them and do not edit them (design D4).
  - Machine-readable taxonomy `platform-core/docs/analytics/taxonomy.v1.yaml` + `TRACKING_TAXONOMY.md` (owned by
    team-analytics) and **ADR-0014** "Tracking event platform" (0013 stays reserved for the placement engine;
    C2 = ADR-0015, C3 = ADR-0016).
- **team-frontend**: a small tracking SDK (queue + batching, `sendBeacon` on page hide, 50%-visible-for-1s
  impression observer, per-request impression dedupe, first-party anonymous-id cookie, opt-out), a `<Placement>`
  wrapper carrying attribution, instrumentation of home rows, search, PDP, rec rows, cart, checkout start and
  share, and `IDENTIFY` after login. No visual redesign (one opt-out toggle in account settings + footer link).
- **team-gateway**: the collector validates each event against the taxonomy, enforces size/batch/field caps and
  per-visitor rate limits, honours opt-out (cookie and `Sec-GPC`), never forwards IP or user agent, stamps server
  time and corrects client clock skew, applies deterministic per-session sampling, and drops invalid events
  individually with reason counters; it also forwards the visitor's computed consent state on every upstream call
  (`x-client-tracking-consent`, rebuilt by the gateway, never trusted from the client) so serving (C3) can stop
  personalising for opted-out visitors; the three `AnalyticsOpsService` RPCs join the admin-only procedure set.
- **team-order** / **team-engagement**: transactional outbox + relayer (the `team-payment` pattern) publishing to
  `order.events` and `engagement.events`.
- **team-analytics**: ingestion of `analytics.events`, `order.events`, `engagement.events`, `payment.events`;
  wide behavioural table + typed fact tables, dedupe by `event_id`, late-arrival window with quarantine, identity
  stitching table, a latest-consent view per subject (`consent_state_latest`, read by C2), retention jobs per PII
  class (the program's single owner of raw-event retention), per-user erasure, and a data-quality gate (schema drift, null
  rates, duplicate rate, volume anomaly vs trailing baseline) exposing metrics, alerts and a status the feature
  store (C2) reads for freshness SLOs.
- **platform-gitops / root compose**: topics, consumer env, retention, alert rules, NetworkPolicies. The analytics
  `DUCKDB_PATH` `/tmp` fix and the analytics PVC are owned by C2 (`analytics-feature-store`); this change only adds
  env next to them.
- **platform-e2e**: every event type proven in the warehouse with shape and attribution; impression→click join on
  `request_id`; opt-out drops events; erasure removes a user's rows.
- No **BREAKING** change: all proto edits are additive; `tracking_events` gains columns append-only; the beacon
  JSON stays backward compatible (old single-object beacons are still accepted).

## Capabilities

### New Capabilities
- `tracking-taxonomy`: the versioned event catalogue (events, per-field definitions, required fields, PII class),
  placement naming, attribution fields and the serving-to-event attribution flow.
- `tracking-client-sdk`: the frontend tracking layer: batching, unload delivery, viewable impressions, dedupe,
  anonymous id, identify, surface instrumentation.
- `tracking-edge-collector`: gateway validation, caps, rate limits, consent, IP stripping, server time and skew,
  sampling.
- `domain-fact-events`: `order.events` and `engagement.events` outboxes and contracts, and the analytics
  consumption of those plus `payment.events`.
- `analytics-ingestion`: warehouse layout, dedupe, late arrivals, identity stitching, inspection read path.
- `tracking-privacy`: anonymous-id rules, no fingerprinting, query scrubbing, retention per class, opt-out,
  erasure, collection documentation.
- `analytics-data-quality`: the data-quality gate, its metrics, alerts and the status consumed downstream.

### Modified Capabilities
- `tracking`: the contract requirement grows from 4 to 11 event types with attribution, and the collector
  requirement changes from "4 types, whole-batch rejection" to taxonomy validation with per-event drops. C2 also
  modifies `tracking` (a different requirement: "The warehouse target is swappable behind a WarehouseWriter seam");
  the two deltas touch disjoint requirements. Archive order: **C1 archives first**, C2 rebases its delta onto the
  result (design "Cross-change ownership").

## Impact

- Repos: `platform-core` (proto, taxonomy, ADR), `team-gateway`, `team-frontend`, `team-analytics`,
  `team-order`, `team-engagement`, `platform-gitops`, root compose, `platform-e2e` + `FEATURES.yaml` in
  `team-analytics`, `team-gateway`, `team-frontend`, `team-order`, `team-engagement`. Coordination only (outside
  this change's code track): `team-ai` and `team-search` populate the new serving response fields (C3 /
  search owners; design D4).
- Contract: cross-service, additive; `buf lint` + `buf breaking` (FILE) must pass. Two new topics
  (`order.events`, `engagement.events`) keyed by aggregate id, `EventEnvelope`-wrapped (Rule 5).
- Architecture rules: Rule 1 (SDK posts only to the gateway), Rule 2 (gateway validates shape, caps, consent and
  strips IP; no analytics logic or storage), Rule 3 (analytics learns order/engagement facts only from topics, never
  from those DBs), Rule 4 (all messages in platform-core), Rule 5 (state changes via outbox → Kafka).
- Data/privacy: more personal data is collected (search, dwell, identify); bounded by PII classes, scrubbing,
  retention, opt-out and erasure (`tracking-privacy`). Legal review of the consent default is a decision item.
- Downstream: C2 (`analytics-feature-store`) consumes the tables, watermarks and data-quality status defined here;
  C3 consumes attribution and the forwarded consent signal. C3 no longer has a raw export (it reads C2 datasets
  only); `tracking_events` columns are only appended.

## Non-goals

- No visual redesign; no consent banner unless the user picks opt-in (design decision 1).
- No feature store, feature definitions or materializers (C2); no model, eval or serving logic (C3).
- No third-party analytics SDKs, pixels, fingerprinting, IP geolocation or session replay.
- No BigQuery production rollout (BigQuery adapter kept compiling and unit-tested behind the seam).
- No A/B experimentation framework (an `experiment` property slot is reserved, nothing assigns variants).
- No serving-side exposure log in team-ai/team-search (client impressions are the exposure log for now).
- No account-deletion flow in team-identity; erasure is an admin RPC (an identity-driven trigger is follow-up).
