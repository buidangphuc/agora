# Design

## Context

`CalculateShippingFee` is a method of `OrderService` (pure, no clock). The `CreateOrder` handler resolves the address inline and swallows errors. `order-multishop-checkout` later reuses the same resolution for `PreviewCheckout` (which reports `shipping_quoted=false` instead of failing), so the resolver is a small handler-level function with a typed result.

## Decisions

- **One city predicate, two rules.** `isMajorCity(city)` is extracted from the fee rule and used by the fee (20.000 vs 35.000) and the ETA (2 vs 4 days). No behaviour change of the fee: the same substring match on the upper-cased city (including its existing quirks) is kept, so fee and ETA can never disagree about what a major city is.
- **Instant on the wire, date in the frontend.** `estimated_delivery_at = now + days x 24 h`. Because HCM has no DST, "today + N calendar days in Asia/Ho_Chi_Minh" equals the dd/mm of this instant formatted in that zone; a late-evening UTC call (already the next day in Vietnam) is a boundary scenario.
- **Injectable clock.** `OrderService` gets a `now func() time.Time` (option, default `time.Now`), used only by the ETA. Existing `time.Now()` calls elsewhere are left alone.
- **Empty city = no quote.** Neither a days value nor an instant is invented; the fee fields keep their current output so no existing caller changes.
- **Fail-closed resolver in the handler.** A function `resolveAddress(ctx, addressID) (*identityv1.Address, error)` returns a gRPC status: `FAILED_PRECONDITION "address not found"` for no match, no addresses, or a resolved address with an empty (blank) city, `UNAVAILABLE` for a client error or nil client. The matching rule is literal: `address_id` set means equal id, empty means `is_default`. The old "first address" fallback is dropped because it can pick an address the buyer neither chose nor saw.
- **Compare before the service call.** The comparison runs in the handler, before `CreateOrdersFromCart`, so nothing is reserved or saga-created. Compared fields are the six delivery fields only; `id`, `user_id` and `is_default` are metadata (a default flag flip is not a change of destination).
- **Same message for unknown and foreign ids.** No existence oracle across buyers.
- **Order of checks in `CreateOrder`:** user principal, kill-switch, idempotency key, address resolution and compare, then the saga. An unauthorised caller never triggers an identity call.
- **No new dependency, no new RPC.** Only the fields added by `q11-checkout-contract`.

## Open points (see OPEN_ISSUES of the slice report)

- Idempotent replay: resolution happens before the saga dedupe, so a replay after the address was edited returns "address changed" instead of the first result.
- Address with an empty `city` at `CreateOrder`: owned by this slice (review round 1). `resolveAddress` refuses it with `FAILED_PRECONDITION` "address not found" (no new message), so `runCheckout` never charges 35000 for city "". `order-multishop-checkout` only reports `shipping_quoted=false` in the preview.
- Upstream deadline: `ListAddresses` gets no deadline of its own in this slice; it is bounded by the caller's request deadline. `UPSTREAM_CALL_TIMEOUT_SECONDS` limits audit writes only.

## Clarifications

### Round 1 — 2026-10-16

- C9 [checkout] Address and ETA: Without a default address an empty address id is refused: A buyer has addresses but none is default and sends an empty address_id — fall back to the first address or refuse? → **FAILED_PRECONDITION "address not found" (no first-address fallback)** (human)
