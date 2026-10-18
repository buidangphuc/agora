## Why

The PDP shipping row promises "Miễn phí vận chuyển cho đơn hàng từ 0Đ", "SPX Express (1-2 ngày)" and an insurance line that no service backs, while the real rule (team-order `CalculateShippingFee`) is free from 500.000đ, else 20.000đ (Hồ Chí Minh, Hà Nội) or 35.000đ. Checkout shows a frontend copy of that rule (`computeShippingFee`), which drifts from the server. A buyer whose cart price went stale (flash sale ended) gets a raw gRPC error instead of the new price (backlog Q5, `buyer-journey.md` #1).

initiative: q5-advertised-price

depends_on: order-advertised-price (team-order refuses CreateOrder with `FAILED_PRECONDITION` and the reason "price changed" and re-prices the cart line; cart unit prices carry flash prices). `OrderService.CalculateShippingFee` and `CreateOrder` are consumed unchanged; no proto or new dependency. team-gateway gains the missing `OrderService.CalculateShippingFee` forwarder (it answered 501 Unimplemented; Clarifications Round 2).

## What Changes

- **PDP shipping row** (`src/app/(shop)/listing/[id]/page.tsx`): replaces the "Vận chuyển" and "Bảo hiểm" rows with one "Vận chuyển" row driven by a new server component `ShippingInfo` (`src/features/listing/ShippingInfo.tsx`). Signed-in buyer with a default address (`isDefault` of `listAddresses()`): "Miễn phí vận chuyển cho đơn từ 500.000đ; dưới mức đó phí giao đến <city> là <fee>" with `<fee>` from `calculateShippingFee(city, listing price below threshold)`. Anonymous visitor, no default address, or a failed server call: "Miễn phí vận chuyển cho đơn từ 500.000đ; dưới mức đó phí từ 20.000đ đến 35.000đ". The insurance row is deleted. The three removed texts ("Miễn phí vận chuyển cho đơn hàng từ 0Đ", "SPX Express (1-2 ngày)", "Bảo hiểm thiết bị và bảo vệ người mua") are added to `FABRICATED_PDP_TEXT` (`src/features/listing/fabricatedText.ts`) so the existing PDP test guards them.
- **Threshold source**: `CalculateShippingFeeResponse` (`shipping_fee`, `is_free_shipping`, `message`) carries no threshold, and a proto change is outside this initiative. Decision: the threshold and the anonymous range stay as named display constants in one module (`src/features/order/shippingCopy.ts`: `FREE_SHIPPING_THRESHOLD = 500000`, `SHIPPING_FEE_RANGE = [20000, 35000]`), and an e2e parity scenario calls the real `CalculateShippingFee` at 500.000 / 499.999 and for both fee tiers so any drift fails CI. Every fee figure for a known city is server data. Raised in OPEN_ISSUES.
- **Below-threshold fee on the PDP**: the PDP asks the server with `items_subtotal = 0` (always below the threshold) so the fee is the below-threshold fee for that city, independent of the price on the page.
- **Gateway wrapper** (`src/lib/gateway/orders.ts`): `calculateShippingFee` returns `null` on error instead of a fabricated 20.000đ "Phí vận chuyển tiêu chuẩn". PDP treats `null` as "no address" (range copy); checkout treats `null` as a hard error (see below).
- **Checkout** (`src/app/(checkout)/checkout/(summary)/page.tsx`): `shipping` = `calculateShippingFee(address.city, cart.subtotal)` mapped to `{ fee, isFree }` for `ShippingStep` / summary. `computeShippingFee`, `FREE_SHIPPING_THRESHOLD` and `src/features/order/shipping.ts` with its test are deleted. If the call returns `null` the page throws to `error.tsx` (a total that omits or invents shipping is never shown). The voucher preview already uses `cart.subtotal` from the server `getCart()` (flash prices included) and keeps doing so; `total = orderTotal(cart.subtotal, discount, shipping.fee)` equals the placed order `total_amount` for a single-shop cart.
- **Price-changed UX** (`src/features/order/actions.ts`, `PlaceOrderForm.tsx`): `checkoutAction` recognises `ConnectError` with `Code.FailedPrecondition` whose `rawMessage` contains `price changed`; it calls `revalidatePath("/cart")` and `revalidatePath("/checkout")` (the cart was re-priced server side) and returns `{ ok: false, error: PRICE_CHANGED_MESSAGE, priceChanged: true }` with `PRICE_CHANGED_MESSAGE = "Giá một số sản phẩm trong giỏ đã thay đổi. Giỏ hàng đã được cập nhật, vui lòng kiểm tra tổng tiền mới rồi đặt hàng lại."`. `PlaceOrderForm` on `priceChanged` releases the lock, shows the message in the existing saga alert slot (info tone) and calls `router.refresh()` so the summary shows the re-priced lines and total; the buyer can press "Đặt hàng" again. No other error is mapped: `UNAVAILABLE`, other FAILED_PRECONDITION reasons and every non price-changed error keep the existing generic error handling and do not revalidate the cart. No order is created on this path (server guarantee), and `purchase` tracking is not fired.
- Tests (colocated vitest) and e2e scenarios in `platform-e2e/tests/e2e/features/frontend/advertised_price_ui.feature` (new file).

## NFR

- PDP: shipping adds at most one `CalculateShippingFee` call plus one `listAddresses` call for signed-in buyers, run inside the existing critical-set `Promise.all`/own Suspense so it never delays first paint of price and gallery; anonymous visitors make no extra call. The shared gateway client (`src/lib/gateway/client.ts`) sets no timeout today, so `calculateShippingFee` passes `{ timeoutMs: 2000 }` (the per-call option pattern of `audit.ts`, which uses 5000); a failing call or one slower than 2 s degrades to the range copy and never errors the page.
- Checkout: one `CalculateShippingFee` per render replacing a local computation; its failure fails the page (availability tied to team-order, which checkout already requires).
- Price-changed: target of 2 s from the refusal to the refreshed total on screen (one `router.refresh()` round trip; non-gated in CI, the e2e only waits for the new total up to its default timeout); the buyer can submit again immediately with a new idempotency key (the attempt signature already includes item prices).
- Money is int64 VND converted with `Number()`, no rounding, as today.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| Default address `city` of the signed-in buyer | personal (location, coarse) | pick the below-threshold shipping fee on the PDP | contract necessity (buyer's own saved address, shown only to that buyer) | not stored by this change; read per request, sent only to team-order over the existing authenticated gateway call, never logged, never put in the URL, a cookie, analytics or the client bundle (server component renders the fee and city name only for that buyer) |
| Cart subtotal, shipping fee, voucher preview | transactional | checkout totals | contract necessity | owned by team-order / team-promotion, unchanged |

No new personal data is collected or persisted. The PDP is `force-dynamic` and per-user; the city appears in HTML only for the authenticated buyer and the page SHALL NOT be cached across users (unchanged `dynamic = "force-dynamic"`).

## Threat model

- Assets: the buyer's default-address city (shown on the PDP), the checkout total, the price-changed signal.
- Abuse: (1) another user's city leaking through a shared cache of the PDP; mitigated by `force-dynamic` and reading the address with the buyer's own token (`listAddresses` is principal-scoped by the gateway). (2) A buyer forcing a wrong total: the displayed total is informational, team-order recomputes and charges server prices; a mismatch is refused, never silently charged. (3) Spoofing the price-changed branch: the client only reads the server error; it cannot make the server create an order. (4) Error text injection: the UI shows a fixed Vietnamese constant for this branch, not the server string.
- Authz rule: PDP shipping personalisation and checkout require the buyer's session; anonymous visitors get the generic copy only; checkout redirects anonymous users to `/login` (unchanged).
