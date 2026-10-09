## Context

Today's code (read from `team-frontend`):

| Route | File | Server / client | State today |
|---|---|---|---|
| `/cart` | `app/cart/page.tsx` (RSC, `force-dynamic`) -> `features/cart/CartView.tsx` | page server; whole view `"use client"` | `useState(initialCart)` copy; `updatingId` |
| `/checkout` | `app/checkout/page.tsx` (RSC; auth redirect, kill-switch, empty-cart redirect) -> `features/order/CheckoutView.tsx` | whole view `"use client"` | `selectedAddressId`, `selectedMethod`, voucher fields, `placing`, `error` in `useState` |
| `/checkout/pay/[id]` | `app/checkout/pay/[id]/page.tsx` (RSC; `notFound()` if no order/payment) -> `features/payment/MockPaymentView.tsx` | whole view `"use client"` | `processing`, `status`, `resultMessage` |

Mutations already are Server Actions with `revalidatePath` (`cart/actions.ts`, `order/actions.ts`
`checkoutAction`, `voucher/actions.ts` `previewVoucherAction`, `address/actions.ts`). `checkoutAction` returns
`{ ok, message, orderIds, paymentUrl }`. Tracking is `trackEcommerce(...)` (`begin_checkout` on mount,
`apply_promotion` on voucher preview, `purchase` after a successful place order); `AddToCartButton` fires
`add_to_cart` and is out of scope. The `data-testid` hooks `voucher-discount` and `order-total` and the
`Xóa tất cả` button label are used by e2e and must be preserved.

Constraints: `ViewCartItem` carries `sellerId`; the shop display name comes from the separate backend change
`shop-display-name` (this change depends on it); there is one voucher code per order; shipping fee is a
client-side display estimate (`computeShippingFee`); there is no idempotency key on `CreateOrder`. Apart from
the shop name, none of these may change (no backend work in this change).

## Goals / Non-Goals

Goals: the three routes composed only from `ui-core-components`, server-first, URL-driven wizard state, a
provably single-submit "Place order", full state coverage (loading/empty/error/not-found), CLS = 0.
Non-goals: see proposal.

## Decisions

1. **Ant Design / Ant Design Pro mapping**

   | Ant Design / Ant Pro | Used for | agora route / component |
   |---|---|---|
   | Pro Form › Step Form | Checkout wizard (Address -> Shipping -> Payment -> Confirm) | `/checkout`: `Stepper` + `CheckoutStepForm` island, `FormItem`, `Radio`, `Button` |
   | Pro Result › Success / Fail | Payment outcome | `/checkout/pay/[id]`: `Result` (success / error / info) |
   | Alert | Saga failure, kill-switch, voucher error, inline errors | `Alert` (type error/warning/info) with action |
   | Card + List | Shop group in cart | `CartGroups` (server): `Card` per shop, header (real shop display name) + rows |
   | InputNumber | Cart quantity | `QuantityPicker` |
   | Modal | Voucher and address selection | `VoucherModal`, `AddressSelectorModal` (`Modal`, `Radio`, `Input`, `Empty`) |
   | Radio.Group (card style) | Payment methods | `PaymentOptionsGrid` (`Radio` + `Card`) |
   | Descriptions | Order summary rows, confirm step review | `OrderSummary` (`Card` + `Descriptions` + `PriceTag`) |
   | Empty | Empty cart, no addresses, no vouchers | `Empty` + action |
   | Skeleton / Spin | Route loading, pending | `loading.tsx` with `Skeleton`; `Spin` inside `Button isLoading` |
   | message | Success / error toasts | `ToastProvider` (`useToast`) |
   | Image | Item thumbnails | `Image` with `aspect-square`, `getImageUrl()` |
   | Tag / Badge | "Mặc định", freeship, voucher chips | `Tag`, `Badge` |
   | Breadcrumb | Cart > Checkout trail on desktop | `Breadcrumb` in the cart header |

2. **Server-first split (§5).**
   - Server components: `/cart` page, `CartGroups` (groups `items` by `sellerId`, renders shop `Card`s, item
     rows, empty state), `OrderSummary`, checkout shell layout, step panels' static content, payment outcome
     `Result` for settled orders.
   - Client leaf islands only: `CartQuantityControl` (`QuantityPicker` + remove), `ClearCartButton`,
     `VoucherModalTrigger`/`VoucherModal`, `AddressSelectorModal`, `PaymentOptionsGrid`,
     `PlaceOrderButton`, `PaymentSimulator` (success/fail buttons), `BeginCheckoutBeacon`.
   - Neither route's page is `"use client"`; `CartView` and `CheckoutView` are removed or reduced to
     composition of the above.

3. **State.**
   - Cart: no client copy of the cart. Each island calls a Server Action, shows pending, and relies on
     `revalidatePath("/cart")` (already in the actions) to refresh the RSC tree; `router.refresh()` is
     not needed.
   - Checkout: `?step=address|shipping|payment|confirm`, `?addr=<id>`, `?pay=<PaymentMethod>`,
     `?voucher=<code>`. Step and selections are therefore back/forward navigable, refresh-safe and
     validated on the server (unknown `step` falls back to `address`; a step whose prerequisites are
     missing redirects to the earliest incomplete step). Only ids and a promo code go in the URL, never
     personal data. Step transitions use `Link`/`router.replace` (no history spam inside a step).
   - Ephemeral only in islands: modal open/closed, in-flight flags.

4. **Mutations return `{ ok, error?, data? }`.** Existing actions return `{ ok, message?, ... }`
   (`CartActionResult`, `OrderActionResult`). The phase normalises them at the island boundary through a thin
   typed wrapper `unwrap(result)` that maps `message` to `error` so islands only handle the §5 shape;
   existing action signatures and their unit tests are kept (additive `error` mirror field if needed). Each
   island: set pending -> call action -> toast success/error -> clear pending.

5. **Double-submit guard (mandatory).** `PlaceOrderButton`:
   - holds `inFlight = useRef(false)`; the click handler returns immediately if `inFlight.current` is true,
     else sets it synchronously before the first `await` (state alone is async and loses a double click in
     the same tick);
   - renders `Button isLoading disabled` while pending (width preserved, `aria-busy`, `aria-disabled`);
   - on `ok:true` it stays locked until navigation completes (does NOT reset in `finally`); on `ok:false` or
     a thrown error it releases the lock and shows the saga `Alert`;
   - also disables the Back link and Stepper navigation while pending;
   - Enter key submit goes through the same handler (the confirm step is a `<form>` with
     `onSubmit` -> same guard).
   Server side nothing changes; the guard is the only protection because there is no idempotency key
   (see Open Questions).

6. **Checkout shell.** `app/checkout/layout.tsx` renders a minimal header (logo link, `Stepper`, "Thanh toán
   an toàn" hint) and a secure footer (policy links) instead of the consumer shell's mega search and bottom
   nav. The shell is a nested layout so `/cart` keeps the consumer shell. Constraint: the root layout still
   mounts `AnalyticsProvider`/`ToastProvider`; the shell only replaces visible chrome. If the root layout
   cannot hide the consumer chrome per segment, use a route group (`app/(consumer)` vs `app/(checkout)`)
   without moving the URL (see Open Questions).

7. **Shipping step.** The only shipping option today is the estimate from `computeShippingFee`. The step
   renders a single pre-selected `Radio` option ("Giao hàng tiêu chuẩn", fee or Freeship `Tag`) so the
   wizard matches Step Form without implying choice that the backend lacks. The helper is moved to a pure
   function in `features/order/shipping.ts` with unit tests; behaviour is identical (>= 500000 free,
   HCM/HN 20000, else 35000).

8. **Responsive.**
   - Desktop (>= 1024px): two columns, `lg:grid-cols-3` (steps 2/3, summary 1/3 sticky `lg:sticky top-4`).
     Stepper horizontal with labels. Cart rows are a table-like grid (thumbnail, title/variant, unit
     price, quantity, line total, remove).
   - Mobile (375px): one column, no horizontal scroll. Stepper compact (numbers + current label). Cart rows
     stack (thumbnail + title, then quantity and line total). The summary is a sticky bottom bar with total
     and primary CTA; the full breakdown opens in a `Drawer`. `Modal`s render as full-height sheets
     (`max-h-[100dvh]`-equivalent token) with an internally scrolling body; tap targets >= 44px.
   - Quantity and primary CTA stay reachable without horizontal scroll at 375px.

9. **CLS = 0.**
   - `app/cart/loading.tsx`, `app/checkout/loading.tsx`, `app/checkout/pay/[id]/loading.tsx` render
     `Skeleton` blocks with the same footprint (shop card, 3 rows, summary card; stepper + step panel;
     `Result` block).
   - Thumbnails use `Image` with `aspect-square` and fixed `w-20`/`w-16`; `getImageUrl()` with fallback.
     Cart item images are below the fold only for long carts: the first shop group is eager, later groups
     `loading="lazy"`.
   - The mobile bottom bar reserves bottom padding on the page body so content does not jump.
   - Voucher preview and shipping updates only change text inside fixed-height rows (the discount row
     always renders, showing "-" when none).
   - The saga `Alert` is rendered in a fixed slot above the step actions, so showing it never moves the
     Place order button.

10. **Type scale and colour (Principle 1).** Only 12/14/16/20/24: `text-xs`, `text-sm`, `text-base`,
    `text-xl`, `text-2xl` tokens. Removes `text-[10px]` and all-caps tracking micro-labels. Brand colour only
    on the primary CTA (`Place order`, `Proceed to checkout`), `PriceTag` totals and `Badge`s. Selected
    cards/radios use neutral border + primary ring token, not brand fill; emoji icons are replaced by
    token icon slots in `PaymentOptionsGrid`.

11. **States.**
    - Cart empty -> `Empty` + "Tiếp tục mua sắm" link to `/`.
    - Cart load failure (gateway error) -> `Alert type=error` + "Thử lại" (`error.tsx` for the segment).
    - Checkout with empty cart -> redirect `/cart` (unchanged); kill-switch off -> `Result` (info) with
      "Quay lại giỏ hàng"; not signed in -> redirect `/login` (unchanged).
    - No addresses -> `Empty` + "Thêm địa chỉ" opening the `AddressModal` form; Next disabled until one
      exists.
    - Voucher invalid -> `FormItem` error + toast; list empty -> `Empty`.
    - `/checkout/pay/[id]` order or payment missing -> `notFound()` -> segment `not-found.tsx` renders
      `Result status=404` with "Xem đơn hàng"; payment fail -> `Result error`; cancelled by saga ->
      `Result` + `Alert` explaining stock released.

12. **Tracking unchanged.** `begin_checkout` fires once per checkout entry from a `BeginCheckoutBeacon` island
    mounted in the shell page (not per step, not on step navigation); `apply_promotion` fires inside the
    voucher island with the same payload (`coupon`, `value`, `properties.valid`); `purchase` fires after
    `ok:true` with the same payload (`transactionId`, `currency`, `value`, `coupon`, `shippingTier`,
    `paymentType`, `items[]`). `data-testid` `voucher-discount`, `order-total` preserved. Recommendation
    placement attribution is not rendered on these routes; nothing is added or removed.

## Risks / Trade-offs

- URL-held wizard state means a stale `?addr=` can reference a deleted address; the server validates against
  `listAddresses()` and falls back to the default address.
- Without an idempotency key, a double submit across tabs or after a network timeout cannot be fully
  prevented client-side; the guard covers the single-click and double-click cases only.
- Grouping by `sellerId` renders the real shop display name from `shop-display-name`; the id-based label
  "Shop #<6 chars>" appears only for an empty name. If the name is resolved by a per-seller lookup, lookups run
  in parallel and one failure falls back to the id label for that shop only.
- No data is invented to fill the cart: no shop stats, ratings or discount figures are rendered.
- Reusing `AddressModal` keeps the address form unchanged but means `AddressSelectorModal` wraps it rather
  than redesigning it (the addresses page belongs to phase 5).

## Open Questions

1. Decided: the cart header renders the real shop display name delivered by the separate backend change
   `shop-display-name`; "Shop #<6 chars of sellerId>" is only the fallback for an empty name.
2. One voucher per order today (`previewVoucherAction(code, subtotal, sellerId of first item)`). For a
   multi-shop cart should the per-shop voucher selector be shown disabled for shops other than the first, or
   should the UI limit to one voucher with the selector on the summary? This spec shows a per-shop selector
   but applies one code; needs product confirmation.
3. Idempotency key on `CreateOrder`: out of scope here (open). A follow-up backend change should add one to
   make the double-submit guard end-to-end; it is listed in the proposal Non-goals.
4. Checkout shell isolation: nested layout vs route group `(checkout)`. Choice depends on how the root
   layout mounts the consumer header; to decide at implementation time.
5. Shipping step has a single option. Keep it as a visible read-only step (this spec), or merge it into
   Payment until a second shipping method exists?

## E2E coverage of the failure-path scenarios

Failure paths are verified through the real stack, never faked: a `@destructive` browser scenario stops (or `docker pause`s, for
"slow") the `agora` compose-project container behind the read and restores it in teardown after the gateway answers again
(`platform-e2e/tests/e2e/support/uif_support.py`; serial lane only). A scenario that cannot be produced through the edge
without fault-injection code in the product is verified by a named Vitest test instead: its delta-spec scenario carries a
`**VERIFIED BY**` line (file + test name) and its FEATURES.yaml entry is `status: not-testable`, the repo's existing exclusion
status. `platform-e2e/scripts/spec_sync.py` does not read that status, so these scenarios still print as uncovered there.

- Real outage (A): "Checkout kill-switch disables the CTA" (`frontend/ui_checkout_order.feature`, flips the stack-wide
  `checkout-enabled` flag and restores it) and "Gateway failure on the cart" (`frontend/uif_cart.feature`, team-order stopped).
- Defect found, kept red: with team-order stopped `/cart` shows "Giỏ hàng của bạn đang trống": `getCart` (`lib/gateway/cart.ts`)
  swallows the failure and returns an empty cart, so `cart/error.tsx` is unreachable and an outage looks like a lost cart.
