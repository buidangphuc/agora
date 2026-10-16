## Decisions

- Threshold from a constant, parity-checked by e2e. The response message is free text and has no threshold field; parsing the message would be a worse copy of the rule. Two probes (threshold free, threshold-1 not free) on the PDP would double calls on every view for one number. A parity e2e scenario keeps the constant honest at CI cost only. If the contract later adds `free_shipping_threshold`, only `shippingCopy.ts` changes.
- PDP asks the fee with `items_subtotal = 0`: the answer is the below-threshold fee for the city regardless of the displayed price (a PDP has no cart; quantity and variant would make the figure misleading).
- `calculateShippingFee` returns `null` on failure rather than a fabricated 20.000đ: the old fallback was exactly the kind of invented number this slice removes. PDP degrades to the range copy; checkout throws because it must not show a total it cannot back.
- Price-changed detection is on `Code.FailedPrecondition` plus `rawMessage` containing `price changed` (the reason fixed by order-advertised-price). `ConnectError.message` is prefixed with the code, so `rawMessage` is the field to read. Other FAILED_PRECONDITION reasons keep the generic error path.
- Refresh strategy: server `revalidatePath` plus client `router.refresh()`; the checkout page is `force-dynamic` and reads the re-priced cart from `getCart()`. A new idempotency key is generated automatically because `attemptKey()` signs `items.map(price)`; the re-priced lines change the signature.
- Multi-shop carts are out of scope (Q11): the "total equals placed total" claim is for a single-shop cart only.

## Clarifications
### Round 1 — 2026-10-16
- Q1 [data/boundary] flash hold lifecycle: TTL, commit point, cancel → **Uncommitted holds expire 15 minutes after reserve (= team-domain reservation TTL). Commit happens in checkout Phase B next to the team-domain CommitReservation (not on OrderPaid). Release returns the units even after commit (stock_sold -= quantity), idempotent; CancelOrder calls Release.** (human)
- Q2 [outcome] reason values → **Exactly the contract set `not_active`, `price_changed`, `insufficient_stock`, checked in that order (first failure wins). Reserve replay of a released or expired hold returns `reserved=false, reason=not_active`.** (default accepted)
- Q3 [outcome] PDP free-shipping threshold source → **Frontend constants (500.000đ, 20.000đ–35.000đ) guarded by an e2e parity scenario against the real CalculateShippingFee; no proto field.** (human)
- Q4 [error] CalculateShippingFee failure at checkout → **Checkout renders the error page (no invented total); the PDP falls back to the range.** (human)
- Q5 [boundary] retry identity → **Every checkout attempt generates fresh order ids, so a reservation_id is never reused after its hold was released.** (default accepted)
