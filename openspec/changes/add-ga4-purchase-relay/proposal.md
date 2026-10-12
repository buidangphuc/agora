# Proposal: GA4 server-side purchase relay

initiative: ga4-analytics

## Why
Browser `purchase` events are lost to adblock and consent denial, so GA4 under-reports revenue-bearing conversions. A server-side worker sends one GA4 Measurement Protocol (MP) `purchase` per paid order. GA4 stays a marketing destination; `order_facts` remains the only revenue source. The first-party `purchase` event (warehouse, recsys) is unchanged.

## What changes
- New worker `team-analytics/cmd/ga4-relay`, a separate binary and consumer group from the warehouse consumer (`cmd/consumer`).
- The relay consumes `analytics.events/TrackingEvent` and keeps `buyer_id -> last ga_client_id` (fields 25/26 from `ga4-consent-contract`); it records a mapping only from events whose `consent.analytics` is true and `ga_client_id` is non-empty, and deletes the buyer's mapping on an event with `consent.analytics=false`.
- On `order.events/OrderPaidEvent` (envelope type `platform.order.v1.OrderPaidEvent`) it POSTs one MP `purchase` (`transaction_id = order_id`, `value`, `currency`, `items`) to the GA4 collect endpoint using plain `net/http`.
- Idempotency: a durable sent-ledger keyed by `order_id` ensures redelivery or restart sends no second event. Ledger and mapping are relay-owned tables (`ga4_relay_sent`, `ga4_relay_client_map`) in the warehouse database, accessed through the team-analytics warehouse adapter; no other service reads or writes them.
- A buyer without a consented GA client id gets no GA4 `purchase`; the relay counts a skip (initiative amendment 2026-10-12).
- GA4 `user_id` is NOT sent by the relay (out of scope).
- Item mapping from `OrderLineItemFact` (which has no name or category): `item_id` = `listing_id`, or `listing_id:variant_id` when `variant_id` is set; `quantity`; `price` = `unit_price`; `currency`. `value` = order `total_amount`, which may differ from the sum of items (shipping, discounts); this is accepted.
- platform-gitops: Deployment `ga4-relay`, ExternalSecret keys `GA4_API_SECRET` and `GA4_MEASUREMENT_ID`, DLQ topic `analytics.ga4-relay.dlq` owned by team-analytics.
- Repos touched: team-analytics, team-frontend (`map.ts`), platform-gitops.
- privacy-reviewer review is mandatory for this change.
- Disabled when `GA4_API_SECRET` is unset (process starts, consumes nothing, sends nothing). `GA4_MEASUREMENT_ID` and `GA4_API_SECRET` come from Vault/ExternalSecrets in platform-gitops.
- team-frontend `src/lib/analytics/map.ts`: the browser stops forwarding `purchase` to GA4/dataLayer; the first-party beacon `purchase` stays.
- No proto change, no new third-party dependency.

## NFR
- Latency: `purchase` sent within 5 min of PAID (p99), measured from `paid_at` to MP 2xx.
- Retry: at most 5 attempts in total (1 initial + 4 retries), exponential backoff base 1 s, factor 2, cap 30 s, only on network error/5xx/429; 4xx other than 429 is not retried. Per-request timeout 5 s.
- Idempotency: at most one successful send per `order_id` across redelivery and restart.
- Availability: relay outage never blocks the warehouse consumer or order flow; offsets are committed only after send, skip or ledger write.
- Limits: mapping store bounded (TTL 30 days per buyer); MP payload under 130 KB, items capped at 200.

## Data classification
| field | class | purpose | consent basis | retention |
|---|---|---|---|---|
| `ga_client_id` (in mapping store and MP `client_id`) | pseudonymous identifier | attribute the server purchase to the GA4 session | analytics consent (`consent.analytics=true` on the source event) | mapping TTL 30 days, refreshed per event; mapping TTL 30 days and ledger 90 days are both <= `tracking_events` retention of 13 months; the mapping is also deleted when consent is withdrawn |
| `buyer_id` (mapping key) | internal pseudonymous id | join PAID order to client id | contract necessity; never leaves the platform | same as mapping |
| `order_id` (MP `transaction_id`, ledger key) | business identifier | dedupe and GA4 transaction key | legitimate interest, shared only when the client id was consented | ledger 90 days (dedupe window >= Kafka retention of `order.events`) |
| `value`, `currency`, item ids/names/qty/price | commercial data | GA4 ecommerce reporting | analytics consent | held by Google per GA4 settings |
| DLQ record on `analytics.ga4-relay.dlq` (verbatim `OrderPaidEvent`: `order_id`, `buyer_id`, amounts, items, `paid_at`; headers `error`, `source_topic`) | business data incl. internal pseudonymous id | let operators inspect and replay orders that failed to reach GA4 (4xx other than 429, retries exhausted, undecodable event) | contract necessity, same as the source `order.events` record | 7 days (`retention.ms` set at create and enforced with `alter-config`); no automatic consumer; not purged early on consent denial or erasure request (Clarifications R2 Q4) |
No `buyer_id`, email, phone, name, address or `user_id` is sent to Google. The DLQ copy adds no new field compared with `order.events`; it extends the life of a failed order's record by at most 7 days, and broker ACLs on the DLQ (ga4-relay / team-analytics only) take effect when broker auth is enabled. Logs carry `order_id` only, never client id or secret.

## Threat model
- Assets: `GA4_API_SECRET`, buyer to client-id mapping, GA4 revenue integrity.
- Abuse cases: forged or replayed `OrderPaidEvent` inflates GA4; duplicate sends via redelivery; secret leaked in logs or URL logs; sending for a buyer who denied consent; forged `ga_client_id` in a beacon body (mitigated: `consent` and `ga_client_id` come only from the edge-stamped event; a buyer can only mislink their own attribution).
- Authz rule: the relay exposes no inbound endpoint other than health/readiness probes. Only the order service's producer credentials may write `order.events` (Kafka ACL), which closes the forged-event surface. Only the relay consumer group credentials read the topics. The secret comes only from env/ExternalSecret, is never logged, and URL query strings are redacted from error logs. Consent is re-checked at relay: no consented client id, no send.
