## Context

Today the cart stores one `unit_price` and checkout trusts it. `redemption.go` already holds a voucher in Phase A, commits on OrderPaid (`consumer/payment.go`) and releases in `failAndCompensate` and `CancelOrder`, keyed by the order id. The flash hold follows the team-domain lifecycle instead (reserve in Phase A, commit in Phase B, release on compensation and cancel), with a key per order line.

## Decisions

- **Stateless price check.** The cart keeps only `unit_price`, and the order item keeps no price source (initiative non-goal). So checkout recomputes P for every line from `GetListing` (R) and `GetActiveFlashSale` and compares with the cart price. "Flash-priced" means P < R. This also catches a seller changing the regular price, which is the same "price the buyer did not see" harm.
- **One rule, two callers.** A single `resolveUnitPrice(listing, variantID, campaign)` pure function is used by `AddToCart` and the checkout pre-check, so they cannot drift. Rule: campaign active, `campaign.variant_id == ""` or equals the line variant, `0 < sale_price < R`.
- **Pre-check before saga.** Running before `CreateSaga` (unkeyed) or through `abandon` (keyed) makes "nothing held" trivially true and frees the idempotency key. A refusal in Phase A (refused hold) uses `fail`, which compensates stock, voucher and earlier flash holds.
- **Refused hold re-prices to regular.** Promotion refuses a hold when the campaign is not active, remaining is short, or the price differs. The buyer then sees the regular price; if the campaign is in fact still active but sold out the regular price is still the right next price, because a sold-out campaign no longer advertises.
- **Hold id** `FlashReservationID(orderID, listingID, variantID)` is `<order id>:<listing id>:<variant id>`. It is derivable from a stored order (`order.ID` + item ids), so commit and release need no new column. The exact shape is owned by `q5-flash-stock-contract`; if that spec fixes another shape only this one function changes.
- **Commit in Phase B, release per item on cancel.** Commit happens in Phase B BEFORE the team-domain commit (committed domain reservations are not known to be releasable, so a flash commit failure must leave domain stock uncommitted; a later domain commit failure releases the committed flash holds, which promotion supports), only for the flash holds recorded in Phase A (known in memory). `failAndCompensate` likewise releases only those recorded flash holds. `CancelOrder` has only the stored order, whose items carry no price source, so it calls release for every item and relies on promotion treating an unknown id as a no-op; this is accepted instead of adding a column. Release after commit returns the units (`stock_sold -= quantity`), so cancelling a placed or paid order restores flash stock, as the human decided.
- **Unpaid orders count as sold.** Because commit is in Phase B, a placed but unpaid order holds sold flash stock until it is cancelled; accepted, unpaid-order expiry is backlog Q20.
- **Fresh ids per attempt.** Order ids are generated per checkout attempt, so a released reservation id is never reused; promotion's replay-of-released answer (`not_active`) is never hit by a retry.
- **Reason values not matched.** team-order treats any `reserved=false` as "price changed" and never branches on `reason` (contract set: `not_active`, `price_changed`, `insufficient_stock`).
- **Promotion client scope.** The flash hold RPCs reuse the `promotion.reserve` service scope by wrapping the flash client like `servicePromotionClient`; `GetActiveFlashSale` stays an unwrapped public read. If `promotion-flash-stock-hold` chooses another scope, only the wrapper constant changes.
- **Cart re-price is a write on refusal.** A refused checkout writes new unit prices, so the buyer's next `GetCart` shows the new total; the frontend slice reads it from there. The error message stays exactly "price changed".
- **Why not split lines:** a line is fully flash or fully regular, so one hold and one unit price per line; the remaining stock check is all-or-nothing.

## Clarifications
### Round 1 — 2026-10-16
- Q1 [data/boundary] flash hold lifecycle: TTL, commit point, cancel → **Uncommitted holds expire 15 minutes after reserve (= team-domain reservation TTL). Commit happens in checkout Phase B next to the team-domain CommitReservation (not on OrderPaid). Release returns the units even after commit (stock_sold -= quantity), idempotent; CancelOrder calls Release.** (human)
- Q2 [outcome] reason values → **Exactly the contract set `not_active`, `price_changed`, `insufficient_stock`, checked in that order (first failure wins). Reserve replay of a released or expired hold returns `reserved=false, reason=not_active`.** (default accepted)
- Q3 [outcome] PDP free-shipping threshold source → **Frontend constants (500.000đ, 20.000đ–35.000đ) guarded by an e2e parity scenario against the real CalculateShippingFee; no proto field.** (human)
- Q4 [error] CalculateShippingFee failure at checkout → **Checkout renders the error page (no invented total); the PDP falls back to the range.** (human)
- Q5 [boundary] retry identity → **Every checkout attempt generates fresh order ids, so a reservation_id is never reused after its hold was released.** (default accepted)

### Round 3 — 2026-10-16 (converge, human)
- Q8 [outcome] e2e VERIFIED BY names → **aligned mechanically to the actual .feature scenario names (renamed during build to echo the spec titles, which spec_sync matches on); FEATURES.yaml covered_by already used them.** (human)
