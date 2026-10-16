## Why
The Q5 initiative charges the buyer the flash-sale price the shop advertised and caps flash-sale stock at the campaign `stock_cap`. `FlashSaleService` has no way to hold stock: `stock_sold` never grows and concurrent checkouts can oversell. This slice adds the three service-only hold RPCs and their messages to the contract, plus the proto copies consumers need. Behaviour lives in `promotion-flash-stock-hold` (team-promotion) and `order-advertised-price` (team-order).

initiative: q5-advertised-price

## What changes
In `platform-core/packages/proto/platform/promotion/v1/promotion.proto`, `service FlashSaleService` gains, additively:
- `rpc ReserveFlashSaleStock(ReserveFlashSaleStockRequest) returns (ReserveFlashSaleStockResponse)`
- `rpc CommitFlashSaleStock(CommitFlashSaleStockRequest) returns (CommitFlashSaleStockResponse)`
- `rpc ReleaseFlashSaleStock(ReleaseFlashSaleStockRequest) returns (ReleaseFlashSaleStockResponse)`

Messages (field numbers fixed):
- `ReserveFlashSaleStockRequest { string reservation_id = 1; string campaign_id = 2; string listing_id = 3; string variant_id = 4; int64 quantity = 5; int64 expected_sale_price = 6; }`
- `ReserveFlashSaleStockResponse { bool reserved = 1; string reason = 2; }`
- `CommitFlashSaleStockRequest { string reservation_id = 1; }`, `CommitFlashSaleStockResponse { bool committed = 1; }`
- `ReleaseFlashSaleStockRequest { string reservation_id = 1; }`, `ReleaseFlashSaleStockResponse { bool released = 1; }`

Documented semantics (in proto comments):
- **Service-only**: for team-order's service principal; not routed through the gateway.
- **reservation_id shape**: `<order_id>:<listing_id>:<variant_id>`, one hold per order line; `variant_id` empty gives `<order_id>:<listing_id>:`. Opaque to team-promotion, non-empty, at most 200 bytes.
- **Closed set of `reason` values** (empty when `reserved=true`): `not_active`, `price_changed`, `insufficient_stock`. Callers treat any other value as a refusal.
- `not_active`: no active campaign with that id, kill-switch off, outside its window, or the campaign does not price that listing/variant. `price_changed`: `expected_sale_price` differs from the campaign `sale_price`. `insufficient_stock`: `stock_cap - stock_sold - holds < quantity`.
- Checks run in the order `not_active`, `price_changed`, `insufficient_stock`; the first failure wins.
- Malformed input is gRPC `INVALID_ARGUMENT`, not a `reason`: empty `reservation_id`, `campaign_id` or `listing_id`, `reservation_id` longer than 200 bytes, or `quantity <= 0`. An empty `variant_id` is valid.
- Lifecycle: an uncommitted hold expires 15 minutes after reserve (the team-domain reservation TTL) and then stops counting against `remaining`. Commit is called in checkout Phase B next to team-domain `CommitReservation`; it is not tied to OrderPaid.
- Idempotency: Reserve with the same `reservation_id` returns the first result without holding twice; a replay after the hold was released or expired returns `reserved=false, reason=not_active`. Commit: `committed=true` when the hold is committed (now or earlier), `false` for an unknown, released or expired id. Release: also returns committed units (`stock_sold -= quantity`); Release of an already released hold, or of an expired uncommitted hold, returns `released=true` and changes no counter; only an unknown id returns `released=false`. A Reserve replay returns the stored outcome and ignores the other request fields. CancelOrder calls Release.
- Callers use fresh order ids on every checkout attempt, so a `reservation_id` is never reused after its hold was released.
- `GetFlashSaleStockResponse.remaining` comment: `stock_cap - stock_sold - active (unexpired, uncommitted) holds` (comment only; no field change).

Copies: `promotion.proto` is copied byte-identical to `proto/platform/promotion/v1/` of team-promotion, team-order, team-gateway, team-analytics and team-frontend. team-order's copy is a stale subset today (it lacks SubscriptionService and SponsoredService); it is replaced by the full source (additive for that consumer). Consumers regenerate in their own slices and never edit the copy.

A committed Go test `platform-core/packages/proto/platform/promotion/v1/flashstock_compat_test.go` (same approach as the analytics `loopevents_compat_test.go`) pins RPCs, messages, field numbers, comment text and wire round-trips.

Out of scope: hold storage and enforcement (promotion-flash-stock-hold), price resolution and saga wiring (order-advertised-price), gateway routes, per-buyer limits, changes to existing messages or RPCs.

## NFR
- Additive only: `buf breaking` against HEAD passes; consumers unaware of the RPCs keep working.
- No new service, port or dependency. Contract-only; no runtime latency impact.
- Compat: a Reserve request written by a new caller parses in an old schema (unknown fields skipped) and old bytes of existing messages are unchanged.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `reservation_id` (order id + listing/variant id) | business identifier, not personal | idempotency key of a stock hold | contract | lives with the hold; defined by promotion-flash-stock-hold |
| `campaign_id`, `listing_id`, `variant_id`, `quantity`, `expected_sale_price` | business data | identify and size the hold | contract | same |
| `reserved`, `reason`, `committed`, `released` | non-personal | outcome | contract | none |

No personal data is added: no buyer id, address or payment data crosses these RPCs.

## Threat model
- Assets: flash-sale stock integrity (`stock_cap` is never exceeded); the price the buyer is charged.
- Abuse: a browser or foreign service reserving or releasing campaign stock (mitigated: service-only, documented here and enforced in promotion-flash-stock-hold, no gateway route); a replayed Reserve double-holding (idempotent on `reservation_id`); a caller passing a forged cheaper price (refused, `price_changed`); a consumer copy drifting from the source (mitigated by the `cmp` check).
- Authz rule: only team-order's service principal with scope `promotion.reserve` may call the three RPCs. This slice documents the rule; enforcement and its tests belong to promotion-flash-stock-hold.
