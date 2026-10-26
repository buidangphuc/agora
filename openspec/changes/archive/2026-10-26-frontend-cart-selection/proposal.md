## Why

Checkout always places the whole cart: the cart page's "Mua hàng" link is `/checkout` with no selection, and the PDP "Mua ngay" adds the variant and then opens `/checkout`, which checks out every line the buyer ever added (backlog Q18, `buyer-journey.md` #10). `CreateOrderRequest.item_ids` already exists and `team-order` removes only the ordered lines from the cart, but the frontend never sends it. A buyer cannot pick lines or shops to pay for, and "Mua ngay" is not "buy now".

initiative: q11-checkout-coherence

depends_on: frontend-checkout-coherence. That slice makes the checkout page read `?items=` and pass it unchanged to `PreviewCheckout.item_ids` and `CreateOrder.item_ids` (it owns `items` in `checkoutParams.ts` / `PlaceOrderForm.tsx`). This slice adds the selection UI on the cart, the true "Mua ngay", the empty-selection redirect and the back link that keeps the selection. No proto change; `CartService.GetCart`, `CartService.AddToCart`, `OrderService.CreateOrder` and `OrderService.PreviewCheckout` are consumed unchanged.

## What Changes

- **Cart selection** (`src/app/(shop)/cart/page.tsx`, new client island `src/features/cart/CartSelection.tsx`, `CartGroups.tsx`): every cart line gets a checkbox and every shop header a select-all checkbox (indeterminate when some lines of the shop are ticked). The page stays a server component (the line rows are still in the first HTML response); only the checkboxes, the summary count/subtotal and the "Mua hàng" link live in the client island. Initial state is "all ticked", or the lines named by `?items=` that exist in the cart (this is how the back link from checkout restores the selection). "Mua hàng" links to `/checkout?items=<id>%2C<id>...` (`URLSearchParams` encoding, same as `checkoutHref`) built from the ticked lines in cart order; it no longer carries the cart's `voucher` (checkout takes vouchers per scope in `sv`/`pv` and ignores a legacy `voucher`, per frontend-checkout-coherence); with nothing ticked it renders as a disabled button.
- **Selection helpers** (new `src/features/cart/selection.ts`): `parseItemsParam(raw)` (comma list, trimmed, de-duplicated, empty parts dropped), `resolveSelection(cartLineIds, raw)` (ids in the cart, in cart order) and `selectionHref(ids, extra)`. Pure functions, unit-tested; the cart island and the checkout page share them so the rule exists once.
- **Cart summary**: item count and subtotal show the ticked lines (sum of the server `unit_price x quantity` already shown per line; no shipping, discount or total is computed here). The discount row of an applied voucher is shown only while every line is ticked; with a partial selection the summary says "Giảm giá được tính ở bước thanh toán" because the checkout quote is the authority.
- **Checkout** (`src/app/(checkout)/checkout/(summary)/page.tsx`): when `items` parses to at least one id, `resolveSelection` against the server cart; none of them in the cart -> `redirect("/cart")`; ids not in the cart are ignored; absent or empty `items` (`?items=`) is the whole cart with an empty `item_ids`, as frontend-checkout-coherence's `checkoutParams` defines; the resolved ids are what the page hands to `PreviewCheckout`, `CreateOrder` and the `begin_checkout` beacon items. "Quay lại giỏ hàng" links to `/cart?items=<resolved ids, %2C-joined>` so the selection survives the round trip.
- **Mua ngay** (`src/features/cart/actions.ts`, `PurchaseContext.tsx`): `addToCartAction` returns `ok({ itemId })`, the id of the cart line matching `listing_id` + `variant_id` in the `AddToCartResponse` cart (AddToCart merges by that pair, so an existing line comes back with its merged quantity). `add(true)` navigates to `/checkout?items=<itemId>` instead of `/checkout`. If the response holds no such line the buyer stays on the PDP with an error toast; it never falls back to the whole cart. Both the inline panel and the mobile `BuyBar` go through the same `add(true)`.
- **Analytics** (`src/lib/analytics/destinations/gtm.ts`): `currentPageLocation` strips `items` as it already strips `returnUrl`, so cart line ids never reach GTM/GA4 `page_location`.
- Tests (colocated vitest) and e2e scenarios in `platform-e2e/tests/e2e/features/frontend/cart_selection_ui.feature` (new file).
- Gateway route check: every RPC this slice calls is already routed by `team-gateway`: `CartService.GetCart` and `AddToCart` (`internal/edge/cart.go`), `OrderService.CreateOrder` (`internal/edge/order.go`). `OrderService.PreviewCheckout` is added to the gateway by order-multishop-checkout (policy entry user/read, forwarder like `CalculateShippingFee`); this slice only forwards `item_ids` to it through frontend-checkout-coherence's wrapper. No gateway task here.

## NFR

- Selection is client state; ticking a box makes no network call and re-renders the summary in one frame (no p95 gate, no server round trip). The cart first response keeps the line rows (existing e2e "Cart page is not a client component" still holds).
- The URL carries cart line ids (UUID, 36 characters each); the selection link SHALL work for at least 50 lines (about 2 KB). Larger selections are not capped by this slice (see OPEN_ISSUES).
- Availability: no new dependency on any service; "Mua ngay" adds one existing `AddToCart` call, as today.
- Checkout with `?items=` places and quotes exactly those lines; the unticked lines remain in the cart after the order (server behaviour of `RemoveItems`, asserted by e2e).

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| Cart line ids in `?items=` | pseudonymous identifier (opaque UUID, meaningless without the buyer's session) | tell checkout which lines to quote and place | contract necessity (the buyer's own cart) | lives in the URL and ordinary access logs of the web tier (existing log retention, unchanged); not stored by this change; stripped from GTM/GA4 `page_location` (requirement "Cart line ids never reach analytics"), and the `begin_checkout` / `purchase` events keep their existing item fields |
| Cart lines, prices, quantities | transactional | render the selection and the summary | contract necessity | owned by team-order, unchanged |

No personal data is added; the URL never holds an address, a name or a voucher code (the cart no longer carries one to checkout).

## Threat model

- Assets: the buyer's cart, the order that is placed from it.
- Abuse: (1) A buyer edits `?items=` to include another user's line id: `resolveSelection` drops ids absent from the buyer's own `GetCart` response, and team-order's `selectItems` only selects among the caller's cart lines, so a foreign id places nothing. (2) A crafted `?items=` that names no line cannot create an order: the page redirects to `/cart`, and a direct `CreateOrder` with such ids is refused by team-order ("cart is empty"). (3) Stale selection (line removed in another tab): the removed id is ignored; if nothing is left the buyer returns to the cart. (4) "Mua ngay" must not widen to the whole cart when the added line cannot be found: it errors instead. (5) A tampered URL cannot change prices: figures come from `PreviewCheckout` / `CreateOrder` on the server.
- Authz rule: cart and checkout require the buyer's session (anonymous users are redirected to `/login`, unchanged); the selection only narrows what the authenticated buyer's own cart yields, it never widens it.
