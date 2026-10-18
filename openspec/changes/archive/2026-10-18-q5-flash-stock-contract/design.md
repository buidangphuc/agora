# Design

## Clarifications
### Round 1 — 2026-10-16
- Field names and numbers follow the Contracts section of `plans/q5-advertised-price/00-spec.md` verbatim (locked).
- Reservation id shape (fixed here): `<order_id>:<listing_id>:<variant_id>`, per order line, colon-separated, variant may be empty. Mirrors the voucher hold, where the caller owns the id; it is opaque to team-promotion, so no parsing contract is added.
- Reason values are a closed set of three lowercase snake_case strings; a string field (not an enum) is kept because the locked contract fixes `string reason = 2`. Unknown or missing campaign maps to `not_active`.
- Refusals are `reserved=false`, not gRPC errors, so team-order can distinguish "price changed" from an outage. Bad input is `INVALID_ARGUMENT`.
- Release of an already released id returns `released=true` (idempotent success); Commit of an already committed id returns `committed=true`. Unknown ids return false and change nothing. (Release of a committed hold and Commit of a released hold are superseded by Round 2.)
- Copies: five repos carry `promotion.proto` (team-promotion, team-order, team-gateway, team-analytics, team-frontend). team-gateway, team-analytics, team-frontend and team-promotion are identical to the source today; team-order's copy is a stale subset and is replaced.
- Test location: `platform-core/packages/proto/platform/promotion/v1/`, dynamic-descriptor approach from the analytics tests (`buf build -o -#format=json`), adding helpers locally as that package is separate.

### Round 2 — 2026-10-16 (spec-clarify, human)
- Q1 [data/boundary] flash hold lifecycle: TTL, commit point, cancel → **Uncommitted holds expire 15 minutes after reserve (= team-domain reservation TTL). Commit happens in checkout Phase B next to the team-domain CommitReservation (not on OrderPaid). Release returns the units even after commit (stock_sold -= quantity), idempotent; CancelOrder calls Release.** (human)
- Q2 [outcome] reason values → **Exactly the contract set `not_active`, `price_changed`, `insufficient_stock`, checked in that order (first failure wins). Reserve replay of a released or expired hold returns `reserved=false, reason=not_active`.** (default accepted)
- Q3 [outcome] PDP free-shipping threshold source → **Frontend constants (500.000đ, 20.000đ–35.000đ) guarded by an e2e parity scenario against the real CalculateShippingFee; no proto field.** (human)
- Q4 [error] CalculateShippingFee failure at checkout → **Checkout renders the error page (no invented total); the PDP falls back to the range.** (human)
- Q5 [boundary] retry identity → **Every checkout attempt generates fresh order ids, so a reservation_id is never reused after its hold was released.** (default accepted)
