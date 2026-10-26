## Why

Two gaps in team-order block coherent checkout (backlog Q11/Q12/Q18, `plans/pm-review-2026-10/buyer-journey.md` #8, #10):

- `CalculateShippingFee` (`internal/service/order.go`, handler `internal/handler/order.go`) quotes only a fee. The buyer is never told when the parcel arrives, so the PDP and checkout cannot show a delivery date.
- The `CreateOrder` handler resolves the shipping address fail-open: an identity error, a missing client, an unknown `address_id` or a buyer with no address all leave `shippingAddr` empty or fall back to the first address, and the order is placed anyway. A buyer can be charged and shipped to an address they did not pick and did not see at confirm.

The locked initiative (`plans/q11-checkout-coherence/00-spec.md`, 2026-10-16) fixes both. This slice is the team-order side of the ETA and the address guard; the contract fields come from `q11-checkout-contract`, the multi-shop quote and voucher work is `order-multishop-checkout` (it builds after this slice on the same files `internal/service/order.go` and `internal/handler/order.go`).

initiative: q11-checkout-coherence

## What Changes

- **team-order, ETA** (`internal/service/order.go`, `internal/handler/order.go`):
  - A mock delivery-time rule sits next to the fee rule: city Hồ Chí Minh or Hà Nội gives 2 days, any other non-empty city gives 4 days (the same city match as the existing fee rule, one shared predicate). An empty city has no rule: no ETA.
  - `OrderService` reads time through an injectable clock (default `time.Now`) so the rule is testable.
  - `CalculateShippingFee` returns `estimated_delivery_days` and `estimated_delivery_at` = clock now + days x 24 h (instant, UTC on the wire). The frontend formats dd/mm in Asia/Ho_Chi_Minh; the server never sends a pre-formatted date. Fee, free-shipping flag and message are unchanged; free shipping does not change the ETA.
  - Empty city: `estimated_delivery_days` 0 and `estimated_delivery_at` unset (never an invented date). The fee fields keep today's value.
- **team-order, address guard** (`internal/handler/order.go`, `CreateOrder`): address resolution becomes fail closed and runs before any reservation, after the user-principal check, the kill-switch and the idempotency-key parse:
  - Source of truth: `AddressService.ListAddresses` with the forwarded buyer principal, so only the buyer's own addresses are candidates.
  - `address_id` set: the address with that id; empty `address_id`: the buyer's default address (`is_default`). No other fallback (the "first address" fallback is removed).
  - No match, or the buyer has no address: `FAILED_PRECONDITION` "address not found". A resolved address whose `city` is empty (after trimming spaces) is refused the same way, so shipping is never quoted or charged for city "" (this slice owns that guard at `CreateOrder`; `order-multishop-checkout` only reports `shipping_quoted=false` in the preview).
  - Identity error, or no address client configured: `UNAVAILABLE` (retryable). An identity outage never places an order.
  - `CreateOrderRequest.confirmed_address` set: compared field by field (`recipient_name`, `phone`, `street`, `ward`, `district`, `city`, exact string equality) with the resolved identity copy; any difference gives `FAILED_PRECONDITION` "address changed". Unset: no comparison (older clients). `id`, `user_id`, `is_default` are not compared.
  - The order keeps storing the resolved identity copy as its delivery snapshot, as today.
  - Every refusal ("address not found", "address changed", `UNAVAILABLE`) is logged with the address id only, never recipient, phone or street.
- **Proto**: `proto/platform/order/v1/order.proto` (and the `identity` import if the contract adds it) copied from `platform-core/packages/proto`, Go stubs regenerated, never hand-edited. Needs `CalculateShippingFeeResponse.estimated_delivery_days/estimated_delivery_at` and `CreateOrderRequest.confirmed_address` from `q11-checkout-contract`.
- **platform-e2e** (owned by this slice): new feature `features/order/address_eta.feature`, steps and an address helper in `support/` (create, edit address through `AddressService` as the buyer).
- Not changed: team-gateway (`CalculateShippingFee` and `CreateOrder` routes exist), team-identity, team-frontend, the shipping fee amounts, `PreviewCheckout`, grouping, vouchers (all `order-multishop-checkout`), team-payment.

## NFR

- Latency: ETA adds no upstream call (pure computation); `CalculateShippingFee` stays in-process, p95 <= 50 ms on the local stack. `CreateOrder` already calls `ListAddresses` once; this slice adds no extra call, only fails closed, no retries. This slice adds no deadline of its own: the call is bounded by the caller's (gateway) request deadline, as today (`UPSTREAM_CALL_TIMEOUT_SECONDS` limits audit writes only and is not applied here).
- Availability: an identity outage makes `CreateOrder` return `UNAVAILABLE` instead of placing the order at an empty address (deliberate trade of availability for correctness). `CalculateShippingFee` does not depend on identity.
- Limits: ETA days are 2 or 4 (mock table); the instant is exact to the second, second-level clock skew is not a requirement.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `confirmed_address` (recipient_name, phone, street, ward, district, city) | personal (buyer contact and location) | detect an address edited after confirm | contract performance (order delivery) | request-scoped: compared and discarded, never stored or logged |
| resolved address snapshot on the order | personal | delivery | contract performance | unchanged (order retention) |
| `CalculateShippingFeeRequest.city` | personal-adjacent (coarse location) | quote fee and ETA | contract performance | request-scoped, not stored |
| `estimated_delivery_at` | operational | delivery promise | same | request-scoped, not stored |

No new personal data is collected or stored. Logs hold address ids only.

## Threat model

- **Assets:** the buyer's address and phone, the placed order's delivery target, the charged amount.
- **Abuse cases:**
  - Order silently placed at an empty or unintended address after an identity outage or an unknown id: removed by fail-closed resolution (`UNAVAILABLE` / "address not found"), nothing reserved.
  - Address edited in another tab between confirm and place (or by a hijacked session) so the parcel goes elsewhere than the buyer saw: "address changed" refuses the order.
  - Buyer passes another buyer's `address_id` to read or ship to it: `ListAddresses` returns only the caller's addresses (forwarded principal), so the id never matches; same "address not found" answer for unknown and foreign ids (no existence oracle).
  - A non-user principal places an order: refused with `PERMISSION_DENIED` before any identity call (existing check kept first).
  - Address leaking through logs or errors: errors carry fixed messages, logs the address id only.
  - ETA abuse: the date is computed server-side from city and clock only; nothing client-supplied but the city.
- **Authz rule:** `CreateOrder` is user-only and acts on the principal's own addresses; `CalculateShippingFee` stays as today (no new data exposed: no per-user information in the response).
