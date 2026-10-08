## Decisions needed from the user

Merged across C1/C2/C3 (deduplicated, with the legal-review items): `plans/ai-first/DECISIONS.md`.

Each item has a recommended default. The specs and tasks are written against the default; changing one changes
only the config value or task noted. Items marked **(legal)** need a privacy/legal owner's sign-off before a
production rollout, not before building.

| # | Decision | Recommended default | Alternative(s) | Touches |
|---|---|---|---|---|
| U1 | Consent model **(legal)** | **Opt-out**: first-party analytics on by default; a privacy notice, an account-settings toggle and a footer link turn it off; `Sec-GPC: 1` is honoured as opt-out. Both modes are built (`TRACK_CONSENT_MODE=opt_out|opt_in`) so switching is config only. Vietnam's Decree 13/2023 and the Personal Data Protection Law may require opt-in consent for behavioural data in production; that is a legal call, not an engineering one | Opt-in banner (no behavioural events until consent; needs one banner component, which the "no redesign" rule otherwise excludes) | D10, tasks 4.x, 3.6 |
| U2 | Retention per PII class **(legal)** | `free_text` (search query, path, referrer, free-text properties): nulled after **90 days**; `pseudonymous` behavioural rows: **13 months**; identity links: 13 months after last seen; `fact` rows: **25 months** (two seasonal years for seller reporting and demand forecasting); quarantine: 14 days; DQ runs: 13 months; Kafka `analytics.events`: 7 days, `order.events`/`engagement.events`: 14 days. **Program owner of raw-event retention**: C2 follows these windows for raw events and only adds its own (offline partitions 180 d, datasets 30 d / 180 d pinned) | Shorter (6 months behavioural) costs year-over-year features; longer raises risk | D11, task 6.2 |
| U3 | Store raw search queries? | **Store scrubbed queries** (emails, phones, keyword-anchored CMND/CCCD masked at the edge), nulled after 90 days; normalized form (lowercased, whitespace-collapsed) alongside | (a) store only a hash + token count (kills query features, query-understanding and LTR later); (b) store raw (rejected: PII) | D9, tasks 3.5, 6.1 |
| U4 | Track anonymous visitors? | **Yes**, with the random first-party `bds_aid` cookie (13 months) and stitching on login; anonymous visitors are most of the traffic and cold-start needs them | Track only logged-in users (recommendations for anonymous visitors stay popularity-only forever) | D6, D7 |
| U5 | Wide vs per-type tables | **Hybrid**: one wide `tracking_events` for client events (append columns) + typed tables for server facts | All per-type (11 client tables), or everything wide (facts squeezed into `properties`) | D8 |
| U6 | Sampling | **No sampling** (all rates 1.0); knobs exist per event type, deterministic per session, `sample_rate` recorded; never sample IDENTIFY / CONSENT_UPDATE / CHECKOUT_START / ADD_TO_CART | Sample impressions at 0.25 from day one (cheaper, but CTR features need reweighting) | D5 |
| U7 | Server-side topics scope | **`order.events`** (OrderPlaced, OrderStatusChanged to PAID/SHIPPED/COMPLETED/CANCELLED) + **`engagement.events`** (FavoriteChanged, ReviewCreated, FollowChanged) + consume existing **`payment.events`** | Add returns/RMA, Q&A, collections, cart server-side facts now (more value, more outbox work); or only orders | D12, groups 7–8 |
| U8 | Who triggers erasure | **Admin RPC** `EraseSubject` (via gateway, `admin` scope) + a runbook; identity-driven account deletion is a follow-up | Build a `UserDeleted` event in team-identity now (identity has no outbox; adds a repo) | D11 |
| U9 | Recommend/SearchListings response fields | **Resolved by the program planner: C1 owns the proto edit** (task 1.4); team-ai (C3) and team-search only populate them; until then the frontend mints `fe-` request ids so C1 e2e is not blocked | — (C3 owning the edit was rejected) | D4 |

## Context

See proposal.md (Why) for the verified gaps. Constraints that shape the approach:

- `TrackingEvent` fields 1–9 exist; `EventEnvelope` fields 1–7. `buf breaking` is FILE-level; everything here is
  additive. `plans/mlops` P1-T1 proposed `placement_id = 10, config_version = 11`; PROGRAM.md (binding) names the
  field `model_version`, so `config_version` is dropped and P1-T1 is superseded by this change.
- The collector (`team-gateway/internal/edge/collector.go`) already resolves the principal from the `session`
  cookie, settles an `X-Request-Id`, caps the body at 64 KiB and produces synchronously with franz-go. The gateway
  computes the client IP for audit metadata (`clientctx.go`) but the collector never copies it into events; that
  stays true and becomes tested.
- `team-analytics` runs one process: the Kafka consumer and the `AnalyticsQueryService` gRPC server share the
  DuckDB handle (single writer). Any new read path (inspection, DQ report) must live in that process.
- `team-payment` has the reference transactional outbox + relayer (`internal/repository/outbox*.go`,
  `internal/events/relayer.go`). `team-order` (migrations up to 0007) and `team-engagement` (up to 0008) have none.
- In-flight changes touching the same files: `recommendations-end-to-end` (C3) adds `SearchListingsRequest.listing_ids
  = 10` (after task 1.4, same file) and `AnalyticsQueryService.GetPlacementMetrics` (after task 1.2, same file);
  `add-hybrid-retrieval-platform` adds `SearchListingsRequest.mode = 9`. Numbers in use after all three:
  `RecommendRequest` 1–5 existing + 6 (C1); `RecommendResponse` 1–2 + 3, 4 (C1); `SearchListingsRequest` 1–8 + 9
  (hybrid) + 10 (C3); `SearchListingsResponse` 1–3 + 4, 5 (C1). No collision.
- Principles applied (user notes, by title): *Data Validation* (validate at ingestion: schema/type, statistical,
  business layers; quarantine, don't silently accept), *Data Leakage* (event time vs ingest time, dedupe so duplicates
  cannot inflate weight or cross splits, time-correct identity stitching), *ML Monitoring & Observability* (L2
  pipeline-health signals: freshness, null rate, volume vs baseline, sliceable per event type).

## Goals / Non-Goals

**Goals:** a contract that every later model and evaluation can rely on (exposure log with attribution); one
taxonomy file that drives gateway validation, warehouse columns and the collection statement; ingestion that is
idempotent and time-correct; privacy controls enforced in code, not policy text; quality signals C2 can gate on.

**Non-Goals (design level):** no serving-side exposure logging; no stream processing engine (plain consumers +
DuckDB); no schema registry service (the taxonomy file + proto are the registry); no per-event JSON schema in the
browser (the SDK is typed, the gateway validates).

## Decisions

### D1. Contract additions (platform-core, additive)

`analytics.proto`

| Item | Number | Notes |
|---|---|---|
| `EVENT_TYPE_PAGE_VIEW` .. `EVENT_TYPE_CONSENT_UPDATE` | 5–11 | `PAGE_VIEW=5, SEARCH=6, REMOVE_FROM_CART=7, CHECKOUT_START=8, SHARE=9, IDENTIFY=10, CONSENT_UPDATE=11` |
| `string placement_id` | 10 | `<surface>.<slot>` |
| `string request_id` | 11 | **serving** request id (not the beacon's `EventEnvelope.request_id`, which stays the HTTP correlation id); comment says so |
| `string model_version` | 12 | from the serving response |
| `uint32 dwell_ms` | 13 | VIEW only |
| `int32 result_count` | 14 | SEARCH only; -1 = unknown |
| `map<string,string> filters` | 15 | SEARCH; keys registered in taxonomy |
| `string client_event_id` | 16 | UUIDv4 minted by SDK |
| `google.protobuf.Timestamp client_occurred_at` | 17 | raw browser time |
| `uint32 quantity` | 18 | ADD/REMOVE_FROM_CART, CHECKOUT_START item count |
| `string share_channel` | 19 | `copy_link|facebook|zalo|messenger|native|other` |
| `double sample_rate` | 20 | 0 means 1.0 (unsampled, pre-C1 rows) |
| `string page_type` | 21 | `home|search|pdp|cart|checkout|shop|account|other` |
| `repeated string listing_ids` | 22 | CHECKOUT_START only |
| `string consent_state` | 23 | CONSENT_UPDATE only: `granted|denied` |

`events.proto`: `uint32 schema_version = 8` on `EventEnvelope` (0 = legacy/unversioned; tracking v1 = 1; domain
facts may leave it 0).

New service in `analytics.proto` (same package, so the gateway forwards it like `AnalyticsQueryService`):
`AnalyticsOpsService { InspectEvents; EraseSubject; GetDataQualityReport }`. Authz: `InspectEvents`,
`EraseSubject` require scope `admin`; `GetDataQualityReport` also requires `admin` (decision 2026-10-02: C2 reads the
DQ verdict in-process through the fsruntime hook, so no service principal needs the RPC; team-analytics@cfcb5f7). At the gateway all three are **admin-only**:
task 3.10 adds them to `adminProcedures` and moves the pinned set from {`QueryAuditLog`, `ReviewKyc`,
`ResolveDispute`} to six entries, and updates AGENTS.md §4; C3 then adds `GetPlacementMetrics` (seven).

`order.proto`: `OrderPlaced { order_id, buyer_id, seller_id, repeated OrderFactItem items {listing_id, sku,
quantity, unit_price}, total, currency, voucher_code_present bool, occurred_at Timestamp }`,
`OrderStatusChanged { order_id, buyer_id, seller_id, OrderStatus from_status, OrderStatus to_status, occurred_at }`.
`engagement.proto`: `FavoriteChanged { user_id, listing_id, bool added, occurred_at }`, `ReviewCreated { review_id,
listing_id, order_id, buyer_id, seller_id, int32 rating, occurred_at }`, `FollowChanged { follower_id, seller_id,
bool following, occurred_at }`.

`recommendation.proto`: `RecommendRequest.placement_id = 6`; `RecommendResponse.request_id = 3`,
`RecommendResponse.placement_id = 4`. `search.proto`: `SearchListingsResponse.request_id = 4`,
`SearchListingsResponse.model_version = 5` (strategy/ranker version, e.g. `bm25@v1`).

Alternative rejected: a separate `ImpressionEvent` message per type (PROGRAM keeps one `TrackingEvent`; the wide
message also keeps the consumer a single code path). Alternative rejected: carry attribution in `properties`
(untyped, unvalidated, invisible to `buf breaking`; P1-T1 already ruled it out).

### D2. Taxonomy as data: `platform-core/docs/analytics/taxonomy.v1.yaml`

One YAML (owned by team-analytics via CODEOWNERS) lists events (name, enum value, purpose, required, optional,
`never_sample`), fields (type, max_len, pattern, PII class, retention class, description), registered placements,
registered `filters`/`properties` keys and the scrubbing rules version. Consumers of the file:

- a `platform-core` check (`scripts/taxonomy_check.py`, stdlib only) that cross-checks it with `analytics.proto`
  and regenerates/diffs `docs/analytics/TRACKING_COLLECTION.md` (the collection statement);
- `team-gateway` vendors the YAML (same `make vendor-proto` flow, embedded with `go:embed`) and builds its validator
  from it at startup; a drift test compares the vendored copy's hash with platform-core's;
- `team-analytics` uses it for null-rate checks (required fields) and retention classes;
- `team-frontend` vendors a generated TS constant of placement ids and event names (type safety only).

Validation therefore lives in data, not in gateway code paths per event type, which keeps the gateway free of
analytics knowledge (Rule 2): it applies a schema, it does not interpret events.

Required fields v1 (beyond `event_type`, `anonymous_id`, `session_id`, `client_event_id`, `page_path`):

| Event | Required | Optional |
|---|---|---|
| PAGE_VIEW | page_type | referrer |
| VIEW | listing_id | dwell_ms, attribution |
| IMPRESSION | listing_id, placement_id, request_id, position>0 | model_version |
| CLICK | listing_id, position>0 (attribution required when placement_id set) | search_query |
| SEARCH | search_query or filters, result_count | request_id |
| ADD_TO_CART / REMOVE_FROM_CART | listing_id, quantity>0 | attribution |
| CHECKOUT_START | listing_ids (1..100), quantity | — |
| SHARE | listing_id, share_channel | — |
| IDENTIFY | (anonymous_id; user from principal) | — |
| CONSENT_UPDATE | consent_state | — |

PII classes: `none` (event_type, listing ids, placement, position, model_version, counts, page_type), `pseudonymous`
(anonymous_id, session_id, request_id, client_event_id, principal id), `free_text` (search_query, page_path,
referrer, free-text properties), `fact` (server facts), `prohibited` (IP, UA, email, phone, name, address, geo —
never collected; listed so the check can refuse them as field names).

Versioning: `v1` covers all of the above. Adding an event/field/placement = additive proto + YAML entry in one
platform-core PR, `schema_version` stays 1 while changes are additive and optional; bump to 2 only when a required
set changes. Consumers accept `<=` known version and count newer.

### D3. Placement registry v1

`home.for_you` (RecommendationsRow, context HOMEPAGE), `home.trending`, `home.recently_viewed` (RecentlyViewedRow),
`home.flash_sale` (FlashSaleSection), `search.results`, `pdp.similar` (context SIMILAR_ITEMS), `cart.cross_sell`
(context CART), `assistant.suggestions` (registered for the AI assistant; instrumented when its UI shows cards).
Placements that do not exist as UI yet (`home.trending`) are registered but only instrumented once rendered.

### D4. Attribution flow

```
SSR page ──Recommend{placement_id, anonymous_id|user}──► gateway ─► team-ai
        ◄──{items, model_version, request_id, placement_id}──
SSR renders <Placement id request_id model_version> around the cards (props only, no DOM change)
browser: PlacementContext → useImpression(card) / TrackLink(card) → events carry attribution
PDP/cart: last-click attribution per listing kept in sessionStorage (30 min) → VIEW/ADD_TO_CART carry it
```

- Search: `SearchListingsResponse.request_id/model_version` when set; otherwise the frontend mints
  `fe-<uuid>` per rendered result page and leaves `model_version` empty (counted by DQ as `unknown`).
- Recommend: same fallback until team-ai (C3) sets `request_id`. Minting in the frontend is UI shaping (an opaque
  correlation id), not business logic.
- Why not the gateway's `X-Request-Id`: it identifies one HTTP call to the gateway, but SSR fan-out issues several
  serving calls per page; each list needs its own id, which only the serving response (or the renderer) knows.
- Ownership (U9, resolved): task 1.4 lands the fields; C3 does not edit them, it depends on 1.4 (its tasks 3.1,
  5.4, 9.1 and its own `search.proto` edit 1.2) and fills them in team-ai; the search owners get a follow-up (12.2).

### D5. Gateway collector pipeline

Order per request: body cap (64 KiB → 413) → parse (single | array | `{sent_at, events[]}`) → consent check (cookie
`bds_consent`, `Sec-GPC`, `TRACK_CONSENT_MODE`) → per-visitor rate limit (token bucket keyed by `anonymous_id`, else
in-memory client address; `TRACK_RATE_EVENTS_PER_MIN` default 600, burst 200; separate from the RPC limiter) →
per-event: validate (taxonomy) → scrub (D9) → time (D5a) → sample → build envelope → produce.

- Event id: `event_id = client_event_id` when it parses as a UUID, else a new UUIDv4 (dedupe of beacon retries).
- Produce: switch from one `ProduceSync` per event to one `ProduceSync` of all records of the request (franz-go
  batch); a produce error stays best-effort (logged + counter, still 204).
- Kafka key stays `session_id → anonymous_id → listing_id`; IDENTIFY keys by `anonymous_id`.
- Consent forwarding (task 3.11): the same consent function (cookie `bds_consent`, `Sec-GPC`, mode) runs in the
  shared request preamble for every upstream RPC and sets metadata `x-client-tracking-consent: granted|denied`,
  rebuilt from gateway-computed values exactly like `x-client-ip` (`gateway-forward-client-context`); an inbound
  header of that name is ignored. team-ai (C3) treats `denied` as non-personalised. SSR calls carry the visitor's
  `bds_consent` cookie and `Sec-GPC` header to the gateway (task 4.3). It is informational (no authorization).
- Metrics: `gateway_track_events_total{type,outcome}`, `gateway_track_dropped_total{reason}`,
  `gateway_track_batch_size` histogram, `gateway_track_clock_skew_seconds` histogram.
- Config (all in `config.Settings` + `.env.example`): `TRACK_CONSENT_MODE`, `TRACK_MAX_EVENTS_PER_BATCH=50`,
  `TRACK_RATE_EVENTS_PER_MIN`, `TRACK_RATE_BURST`, `TRACK_LATE_BOUND_HOURS=72`, `TRACK_MAX_SKEW_HOURS=24`,
  `TRACK_SAMPLE_RATES` (`impression=1.0,page_view=1.0,...`), `TRACK_TAXONOMY_PATH` (embedded default).

**D5a time:** `received_at` = gateway clock. If `sent_at` and `client_occurred_at` are present and
`|received_at - sent_at| <= 24h`: `occurred_at = received_at - max(0, sent_at - client_occurred_at)`; else
`occurred_at = received_at`. Never later than `received_at`. Older than 72h → `too_old`. This uses the client only
for the *relative* age of an event inside its own queue, which is immune to absolute clock error.

### D6. Frontend SDK (`src/lib/tracking/`)

- `queue.ts` (in-memory, max 200 events, drop-oldest + counter), flush on 20 events / 5 s / `visibilitychange:hidden`
  / `pagehide` with `sendBeacon` (fallback `fetch keepalive`), body `{sent_at, events}`; `text/plain` keeps it a
  CORS-simple request as today.
- `ids.ts`: `bds_aid` cookie (UUIDv4, 13 months, Lax, Secure outside local) adopting the old `localStorage` id once;
  `bds_sid` in `sessionStorage` with a 30-minute inactivity rotation; SSR reads `bds_aid` via `next/headers` and
  passes it as `RecommendRequest.anonymous_id` for anonymous visitors.
- `consent.ts`: reads `bds_consent` and `navigator.globalPrivacyControl`; when off, `track()` is a no-op except
  `CONSENT_UPDATE`.
- `impressions.ts`: one shared `IntersectionObserver` (threshold 0.5) + 1 s dwell timer per element; a
  `Set<request_id|listing_id>` per page view for dedupe; disconnect on unmount.
- React layer: `<Placement id requestId modelVersion>` context provider, `useImpression(ref, {listingId,
  position})`, `TrackLink` reads the context (existing API kept), `useDwell(listingId)` for PDP, `identify()` called
  from the login/register success path (client side, after the session cookie is set).
- Existing `track()` keeps its signature (adapter over the queue), so current call sites compile unchanged.

### D7. Identity stitching

`identity_links(anonymous_id, user_id, first_seen_at, last_seen_at, identify_count)` upserted from IDENTIFY events
whose principal type is `user`. View `tracking_events_resolved` adds `subject_id`:
user principal → user id; else if the anonymous id links to exactly one user → that user id; else
`anon:<anonymous_id>`. Stitching is retroactive (pre-login events of a single-user device resolve to the user),
which is what cold-start features want; C2's point-in-time builder must use `identity_links.first_seen_at` to avoid
leaking a future login into a past training example (noted for C2, *Data Leakage*).

### D8. Warehouse layout (decision U5)

- `tracking_events` (wide, client events): existing 13 columns + appended `placement_id, request_id, model_version,
  dwell_ms, result_count, filters JSON, client_event_id, client_occurred_at, quantity, share_channel, sample_rate,
  page_type, listing_ids JSON, consent_state, schema_version`. `ingested_at` is **owned by C2** (its task 3.1 and
  its `tracking` delta); whichever change lands first appends it, the other rebases and keeps one column. Why wide: the 11 client types share
  ~80% of columns, feature SQL filters by `event_type` and joins impressions↔clicks inside one table, DuckDB/BigQuery
  are columnar so null columns cost almost nothing, and C3's export keeps one source. Per-type tables would turn
  every funnel/CTR query into a UNION and multiply DDL parity tests by 11.
- Typed fact tables: `order_facts`, `order_fact_items`, `order_status_changes`, `favorite_facts`, `review_facts`,
  `follow_facts`, `payment_facts`. Why typed: different keys, shapes and retention class; they are business records
  with different consumers (seller reporting, demand forecasting) and must not be filterable as "behaviour".
- Support tables: `identity_links`, `ingest_quarantine(event_id, topic, partition, offset, reason, raw_type,
  received_at)`, `ingest_watermarks(table_name, watermark, updated_at)`, `erasure_log`, `erasure_tombstones
  (subject_hash)`, `retention_runs`, `dq_runs`, `dq_results`.
- Every table's DDL derives from a `[]Column` list in `internal/warehouse` (extends the existing parity anchor).
  Migration of `tracking_events`: `ALTER TABLE ADD COLUMN` per new column (idempotent), then a one-time dedupe
  (`CREATE TABLE … AS SELECT … QUALIFY row_number() OVER (PARTITION BY event_id) = 1`, swap), then a unique index
  on `event_id`.
- Writes: DuckDB `INSERT … ON CONFLICT (event_id) DO NOTHING`; the dropped-count is returned so the DQ duplicate
  rate is measured, not guessed. BigQuery adapter: `insertId = event_id` (best-effort) + `*_dedup` views
  (`QUALIFY row_number()`); downstream reads views.
- Topic consumers: one consumer group per topic (`team-analytics.tracking`, `.order`, `.engagement`, `.payment`),
  each with a type switch → mapper → batcher → writer; offsets committed after the write (existing discipline).
- Late window `ANALYTICS_LATE_WINDOW_HOURS=72` (matches the gateway bound); quarantine on outside-window and decode
  errors. Watermark = `max(occurred_at seen) - late window`, monotonic, persisted and exported as a gauge.

### D9. Scrubbing

Port `team-ai/app/core/redaction.py` rules (email, VN phone, keyword-anchored CMND/CCCD, bearer/`sk-` secrets) to
Go in `team-gateway/internal/edge/scrub.go` with the **same golden test vectors** (a shared
`platform-core/docs/analytics/redaction-vectors.json` both repos test against, so the rules cannot drift).
National-id masking is on for queries (keyword-anchored, so prices are untouched). `page_path`: drop `?…` and
`#…`; `referrer`: same-site → path only, external → host only. Applied before produce: unscrubbed text never
reaches Kafka (*Data Validation*: validate at ingestion).

### D10. Consent and opt-out

`TRACK_CONSENT_MODE` (gateway + `NEXT_PUBLIC_TRACK_CONSENT_MODE`): `opt_out` (default, U1) treats absence of a
choice as granted; `opt_in` treats absence as denied. Denied ⇒ SDK stops; gateway drops all but CONSENT_UPDATE.
Server-side facts (orders, favorites, reviews, follows) are business records, not tracking; they keep flowing, and
C2 must exclude opted-out users from personalization features and datasets by reading the latest CONSENT_UPDATE per
subject (view `consent_state_latest(subject_key, consent_state, occurred_at, ingested_at)`, task 5.5; `subject_key`
= user id for a user principal, else `anon:<anonymous_id>`; a user-principal update also writes the row for its
`anon:` key). Serving (C3) stops personalising through the forwarded consent header (D5). UI: one toggle in `/account` (privacy section) and a footer link that toggles and
explains; no banner in `opt_out` mode.

### D11. Retention and erasure

`internal/retention` runs daily (`RETENTION_ENABLED`, windows from env, defaults per U2), each step one SQL
statement per table, recorded in `retention_runs`. `EraseSubject(user_id)` runs in one DuckDB transaction: collect
linked anonymous ids → delete from `tracking_events` (principal or linked anon), `identity_links`; in fact tables
replace buyer/user ids with `erased:<hmac(user_id)>` (seller revenue totals stay correct) and delete
favorite/follow facts; insert `erasure_tombstones(hmac(user_id))` and each linked `hmac(anonymous_id)`; record in
`erasure_log`. The consumer checks tombstones before insert (in-memory set, reloaded on change), so replays cannot
restore erased rows. After the transaction `EraseSubject` calls the feature-store erasure hook registered by C2 (its
task 8.4: online keys, partition rewrites, dataset invalidation); C2 does not delete raw rows itself once this RPC
exists. C2 datasets filter tombstones (stated in the C2 handoff; Kafka retention bounds raw residue to 7–14 days).
The retention job is the single owner of raw-event retention for the program (U2); C2's sweep never deletes raw rows.

### D12. Domain fact outboxes (team-order, team-engagement)

Copy the team-payment shape: `outbox` table (migration `0008_outbox` in team-order, `0009_outbox` in
team-engagement), `OutboxWriter` used inside the existing repository transactions, a relayer goroutine started from
`main` when `KAFKA_ENABLED=true` (poll, produce, mark sent, at-least-once), topic env `ORDER_EVENTS_TOPIC` /
`ENGAGEMENT_EVENTS_TOPIC`. Order emission points: `CreateOrder` (saga commit), PaymentSettled consumer (→PAID),
`UpdateOrderStatus` (→SHIPPED/COMPLETED), `CancelOrder`/saga compensation (→CANCELLED). Engagement: AddFavorite /
RemoveFavorite only when a row changed, CreateReview, FollowSeller / UnfollowSeller only when a row changed. If a
repository runs in in-memory mode (no DB), the outbox is in-memory and the relayer still publishes (dev only).

### D13. Data-quality gate

`internal/dq` runs every `DQ_INTERVAL=15m` over the last complete hour, per table × event type: required-field null
rate (from taxonomy), duplicate rate (from writer counters), quarantine rate, schema-ahead count, volume vs the
median of the same hour-of-day over the trailing 7 days (fail below 0.3× or above 5×; `insufficient_baseline`
below 7 days), attribution completeness for impressions/clicks. Thresholds in `dq.yaml` (embedded, overridable).
Results go to `dq_runs`/`dq_results`, Prometheus gauges `analytics_dq_status{table,event_type,check}`,
`analytics_dq_value{…}`, `analytics_ingest_watermark_seconds{table}`, and `GetDataQualityReport`. Alert rules
(gitops + local Prometheus rule file): `AnalyticsDataQualityFailing` (fail for 2 runs), `AnalyticsIngestLagHigh`
(watermark older than 2× late window), `GatewayTrackDropRateHigh` (drops/accepted > 5% for 15 m). C2 contract:
materializers read status + watermark per table/type and refuse to advance a feature's freshness past a `fail`
window (that SLO is C2's; this change only provides the signal).

### D14. Cross-change ownership and archive order (program resolutions)

- Serving attribution proto fields: this change (1.4). C3 fills them; C3's `analytics.proto` edit (`GetPlacementMetrics`)
  needs 1.2, its `search.proto` edit (`listing_ids = 10`) needs 1.4.
- Admin-only set: task 3.10 here (AGENTS.md §4 + gateway pinned test); C3 3.2 appends its RPC afterwards.
- Raw-event retention: this change (U2, task 6.2). Compose `DUCKDB_PATH` and the analytics PVC: C2 (its 9.1 / 10.1);
  tasks 10.2 and 11.1 here only add env and need those.
- `ingested_at` column: C2 (D8).
- `tracking` main spec: this change modifies "Analytics tracking events are defined in the contract" and "The edge
  collector accepts browsing beacons and produces tracking events"; C2 modifies only "The warehouse target is
  swappable behind a WarehouseWriter seam". Disjoint requirements, so no requirement text is edited twice. Archive
  order: **C1 archives first**; C2 re-runs `openspec validate --strict` against the archived spec and rebases if
  needed. Neither change may add a delta for the other's requirement.
- ADR numbers: C1 = ADR-0014, C2 = ADR-0015, C3 = ADR-0016; 0013 reserved (placement engine).

## Risks / Trade-offs

- [Event volume grows 10–50× (impressions)] → batching, per-visitor limits, sampling knobs, DuckDB columnar; DQ
  volume check catches runaway loops.
- [Client-minted `fe-` request ids hide missing serving ids] → DQ counts `fe-` share per placement; C3 removes them.
- [Retroactive stitching leaks future identity into past rows] → `first_seen_at` kept for point-in-time joins (C2).
- [DuckDB single writer: erasure, retention and DQ compete with ingestion] → all run in the consumer process under
  the same handle, scheduled between batches; erasure holds a short transaction.
- [Opt-out default may be non-compliant in production] → mode switch built and tested; legal decision U1 before
  prod rollout.
- [Taxonomy drift between platform-core and the vendored gateway copy] → hash drift test + repo-doctor check.
- [Outbox relayer down ⇒ facts delayed] → outbox rows persist; relayer lag metric and alert.
- [Scrubbing false negatives (unusual phone formats)] → shared golden vectors, 90-day null-out bounds exposure.

## Migration Plan

1. platform-core proto + taxonomy (group 1), regenerate everywhere (group 2). Old producers/consumers keep working.
2. team-analytics schema migration (append columns, dedupe, unique index) ships before the gateway sends new fields;
   unknown new fields are simply stored once the consumer knows them.
3. Gateway collector accepts both the old single-object beacon and the new batch shape; deploy before the frontend.
4. Frontend SDK + instrumentation; old `localStorage` ids adopted into the cookie.
5. Outboxes (order, engagement) and analytics fact consumers; topics created first (compose `redpanda-init`, gitops).
6. Retention/erasure/DQ enabled last (`RETENTION_ENABLED`, `DQ_ENABLED` default true in compose, false until
   verified in gitops).
Rollback: each step is independently revertible; reverting the frontend returns to unbatched beacons that the new
collector still accepts; columns stay (append-only).

## Open Questions

- Exact DQ thresholds per event type after a week of real traffic (defaults in `dq.yaml` are a starting point).
- Whether `home.trending` gets a serving source in C3 or stays registered-but-unused.
