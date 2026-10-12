# 00-spec — GA4 tracking feeding analytics and recommendations

initiative: ga4-analytics

## Goal
Buyers choose, through a consent banner, whether Agora may use analytics cookies, marketing cookies
and behaviour-based personalisation. The choice is enforced end to end: GA4 loads only after
analytics consent, and recsys learns only from buyers who allowed personalisation.
Marketing sees acquisition and ecommerce funnels in GA4, with server-side purchases so adblock does
not lose revenue events. Sellers and ops see funnels split by acquisition channel. Recsys uses list
position and campaign context as ranker features.

## User stories
- **Consent.** Given a first visit, then the banner shows and no request goes to googletagmanager.com
  or google-analytics.com.
  When the buyer accepts analytics, then GTM loads and dataLayer receives `consent update` with
  analytics_storage=granted.
  Edge case: the buyer rejects everything. First-party beacons still flow (necessary, operational
  stats), but those events are flagged as personalisation-denied.
- **Personalisation opt-out.** Given a buyer who denied personalisation, when recsys trains, then
  none of that buyer's events appear in interactions, and they get popular/contextual
  recommendations instead.
  Edge case: the buyer later grants consent. Only events after the grant are used.
- **Server purchase.** Given an order reaching PAID for a buyer with a consented GA client id, then exactly one GA4 `purchase` with
  `transaction_id=order_id` is sent through Measurement Protocol, also when Kafka redelivers.
  The browser no longer sends `purchase` to GA4.
  Edge case: the buyer has no consented GA client id. Nothing is sent and the relay counts a skip.
- **Acquisition funnel.** Given GA4 export sessions and first-party events joined through the GA
  client id, when an ops user opens the funnel, then counts are split by source/medium/campaign.
- **Reconciliation.** When daily GA4 event counts differ from first-party counts by more than the
  configured threshold, then a data-quality alert fires.

## Non-goals
- GA4 is never the source of revenue. `order_facts` is the only source.
- GA4 is never the training source for recsys. The first-party pipeline stays the source of truth.
- Meta CAPI, Google Ads audiences and server-side GTM containers are out of scope.
- No consent for logged-in account settings sync across devices (cookie-scoped only in v1).
- No GA4 in dev/CI/e2e. IDs are empty there, and the GA4 import uses a fixture.

## Architecture decisions
- **Two pipelines, separate roles.** Why: GA4 loses 10-30% of events (adblock and consent), has no
  per-slot impressions and exports with about a day of delay. First-party has none of these
  problems but lacks acquisition data.
- **Consent Mode v2, basic mode.** GTM is not loaded before analytics consent. Why: Decree
  13/2023 requires explicit consent before collecting personal data, and basic mode sends no
  cookieless pings.
- **Consent is stored in the first-party cookie `agora_consent`** as
  `v=1&a=0|1&m=0|1&p=0|1&ts=<unix>` (amended 2026-10-12: `;` cannot appear in a cookie value). The gateway stamps it onto events. Why: one source of truth
  that is readable at the edge, needs no backend store, and keeps the gateway dumb (it only
  parses a cookie).
- **GA4 `user_id` is an HMAC-SHA256 of the user id**, with the key held in the frontend server
  env. Why: no PII goes to Google, and the id stays stable across devices.
- **The `ga4-relay` worker lives in team-analytics as a separate cmd.** It keeps
  `buyer_id → last ga_client_id` from `analytics.events` and sends `purchase` on `OrderPaidEvent`.
  Why: no order contract change is needed. The relay is a destination, not part of the
  warehouse consumer.
- **The GA4 export is imported into the warehouse** as `ga4_sessions`. Locally this is a Parquet
  fixture with the GA4 export schema. Why: joins and reconciliation run on the existing
  DuckDB/BigQuery adapters.
- No new third-party runtime dependency except the GA4 MP HTTP endpoint (plain net/http) and the
  BigQuery client that team-analytics already has.

## Contracts
```proto
// platform/analytics/v1/analytics.proto — add to message TrackingEvent (additive)
string ga_client_id = 25;            // value of the _ga cookie (client id part); empty without analytics consent
ConsentState consent = 26;           // stamped by the edge from the agora_consent cookie, never from the body

message ConsentState {
  bool analytics = 1;
  bool marketing = 2;
  bool personalization = 3;
  int32 version = 4;                 // 0 = no consent cookie present (treated as all denied)
}
```
```proto
// platform/analytics/v1 — new admin-only RPC on AnalyticsQueryService (amended 2026-10-12)
rpc GetChannelFunnel(GetChannelFunnelRequest) returns (GetChannelFunnelResponse);
// request: date range (+ optional seller_id); response: rows of {source, medium, campaign, funnel step counts}
```
Existing contracts consumed unchanged: `analytics.events/TrackingEvent`,
`order.events/OrderPaidEvent`, `AnalyticsQueryService.GetSellerFunnel`.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| consent + GA client id on events | ga4-consent-contract | platform-core | – | provides: TrackingEvent.ga_client_id, TrackingEvent.consent, ConsentState | platform-core |
| consent banner, Consent Mode v2, edge stamping | add-consent-mode | team-frontend | ga4-consent-contract | uses: analytics.events/TrackingEvent | team-frontend, team-gateway |
| GA4 destination (GTM container, pseudonymous user_id, CSP, ga_client_id in beacons) | add-ga4-destination | team-frontend | add-consent-mode | uses: analytics.events/TrackingEvent | team-frontend |
| server-side purchase via Measurement Protocol | add-ga4-purchase-relay | team-analytics | ga4-consent-contract | uses: order.events/OrderPaidEvent, analytics.events/TrackingEvent | team-analytics, team-frontend |
| GA4 export import, attributed funnel, reconciliation alert | add-ga4-export-import | team-analytics | ga4-consent-contract | uses: analytics.events/TrackingEvent | team-analytics, platform-gitops |
| recsys: consent filter, position debias, campaign context, nearline checkout signals | recsys-consent-and-context | platform-recsys | add-ga4-export-import | uses: analytics.events/TrackingEvent | platform-recsys, platform-featurestore, team-analytics (export) |

Hot files edited by more than one slice (initiative-build serialises those slices):
- `team-frontend/src/lib/analytics/destinations/gtm.ts`, `src/features/tracking/AnalyticsProvider.tsx` — add-consent-mode, add-ga4-destination
- `team-analytics/internal/export/export.go` — add-ga4-export-import, recsys-consent-and-context
- `team-frontend/src/lib/analytics/map.ts` — add-ga4-destination, add-ga4-purchase-relay (stops sending browser `purchase` to GA4)

## Out of initiative
Finish the open change `add-ga4-gtm-datalayer`; this is a prerequisite and is done before
initiative-build. Its tasks are mostly implemented but still unticked. Remaining work:
- BigQuery adapter `ADD COLUMN IF NOT EXISTS` + `ga4_events` view, with parity with DuckDB.
- Export of the ecommerce columns (`item_list_id`, `position`, `event_group_id`,
  `transaction_id`, `currency`, `value`) to Parquet.
- Featurestore/recsys reads those columns tolerantly; `purchase` dedupes by `transaction_id`.
- e2e: dataLayer `ecommerce.items`, one request per list, N items produce N rows with the same
  `event_group_id`, and no GTM when the ID is empty.
- Tick the tasks with test evidence, then archive.

## NFR and data
- **NFR:**
  - The banner must not shift layout (CLS < 0.1) and must not block first render.
  - GTM loads at most 1 extra script before consent: 0.
  - The relay sends a `purchase` within 5 min of PAID, and retries are idempotent per `transaction_id`.
  - The GA4 import runs daily and is idempotent per export date.
- **Personal data:**
  - `ga_client_id`: pseudonymous identifier, used for attribution. Basis: analytics consent.
    Retention follows `tracking_events` retention.
  - `consent` state: proof of consent; kept with each event.
  - GA4 `user_id`: an HMAC, so pseudonymous; sent only with analytics consent.
  - No email, phone or name in any payload.
- **Risk routing expected:** pii, proto. `privacy-reviewer` is mandatory for add-consent-mode,
  add-ga4-destination and add-ga4-purchase-relay.

## Conventions
- FE tests are colocated `*.test.ts(x)` (vitest). Mirror `lib/analytics/queue.test.ts`.
- Gateway: follow `team-gateway/internal/edge/collector_test.go`. The edge only parses the cookie
  and stamps it; no business logic (AGENTS.md Rule 2).
- Warehouse changes go through `warehouse.Schema` plus the DuckDB↔BigQuery parity test. Migrations
  use `ADD COLUMN IF NOT EXISTS`.
- Recsys reads new columns only when present (`present = [c for c in COLS if c in df.columns]`).
- Secrets (`GA4_API_SECRET`, `GA4_USER_ID_HMAC_KEY`) come via Vault/ExternalSecrets in
  `platform-gitops`. Everything is disabled when unset.
- e2e: `platform-e2e/tests/e2e/features/tracking/`. Assert no external network to Google in CI.

## Amendments 2026-10-12 (step 4d, confirmed by Bùi Đăng Phúc)
- Cookie `agora_consent` uses `&` separators: `v=1&a=..&m=..&p=..&ts=..`.
- Channel funnel: new admin-only `AnalyticsQueryService.GetChannelFunnel`, added by `ga4-consent-contract`; used by `add-ga4-export-import`.
- Recsys: no request-time campaign boost (RecommendRequest unchanged). Campaign context is a ranker training feature only; cold-start keeps the existing popular/contextual path.
- Purchase relay: a buyer without a consented GA client id gets no GA4 `purchase`; the relay counts a skip.
- Defaults taken by the orchestrator (human may override at LOCK): `ga_client_id` with analytics=false is blanked, the event is kept; `ga_client_id` stores the client-id part without the `GA1.1.` prefix; tracking_events retention = 13 months and the new fields follow it.

## Locked
2026-10-11 — Confirmed by Bùi Đăng Phúc. 6 slices; first-party pipeline is the source of truth for recsys/analytics, GA4 is a marketing destination plus acquisition import; Consent Mode v2 basic via agora_consent cookie; add-ga4-gtm-datalayer finished first, outside the initiative.
