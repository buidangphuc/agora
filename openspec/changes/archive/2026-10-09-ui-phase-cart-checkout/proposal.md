## Why

Phase 3 of the UI system (`UI_SYSTEM_DESIGN.md` §6) covers the money path: `/cart`, `/checkout` and
`/checkout/pay/[id]`. Today they are three client-heavy screens built before the core component set:

- `CartView` is a `"use client"` page that copies server data into `useState`, is flat (no grouping by shop),
  swallows failed mutations silently, and has no voucher entry.
- `CheckoutView` is a single long page with inline radios, an inline `<input>` voucher box, hard-coded
  emoji icons, raw `text-[10px]` and `text-xs` micro-text, and an error string instead of an Alert. Its
  `placing` flag is the only double-submit protection and is reset in `finally`, so a fast second click after
  a successful order can create a second order.
- `MockPaymentView` shows the payment result as plain text and has no failure recovery.

Principle 2 (Unbreakable & Predictable Commerce) requires pending, disabled and success/error feedback on
every transaction touchpoint. This change rebuilds these three routes from the `ui-core-components` set,
following the Ant Design Pro **Step Form** (checkout), **Result** (payment outcome) and **Alert** (saga
failure) templates, without changing any data flow.

## What Changes

- `/cart`: server-rendered, grouped by shop (shop header with the real shop display name, item rows with `QuantityPicker`, per-shop voucher
  selector opening `VoucherModal`), sticky order summary, `Empty` state. Quantity, remove and clear are
  Server Actions with pending, disabled and toast feedback; the page no longer mirrors server data in
  client state.
- `/checkout`: a distraction-free **checkout shell** (stepper header, secure footer, no mega nav) with a
  4-step `Stepper` Address -> Shipping -> Payment -> Confirm. The current step and selections live in URL
  searchParams. New leaf islands: `AddressSelectorModal`, `VoucherModal`, `PaymentOptionsGrid`.
- A persistent **order summary** (`Card` + `Descriptions` + `PriceTag`) with subtotal, voucher discount,
  shipping fee and total.
- **Mandatory double-submit guard** on "Place order": the button is disabled and `isLoading` while pending,
  protected by a synchronous in-flight lock, and stays disabled after success until navigation. One click
  creates one order.
- Saga failure surfaced as `Alert` with a recovery action; `/checkout/pay/[id]` outcome rendered with
  `Result` (success, payment failed, order cancelled by saga compensation, not found).
- Only real data is shown: the shop header uses the real shop display name (fallback "Shop #<6 chars>" only
  for an empty name); no invented shop stats, ratings, badges or discount figures in the cart or checkout.
- Route-level `loading.tsx` skeletons for all three routes (CLS = 0).

Repos: `team-frontend` (code), `platform-e2e` (e2e track).

## Capabilities

### New Capabilities
- `ui-cart-checkout`: cart, checkout wizard, order summary and payment-outcome screens built from the core
  component set.

### Modified Capabilities
None. The existing e2e scenarios (`cart.item-management`, `promo` voucher redemption, `mock_pay`,
`saga_compensation`, empty cart state) keep their behaviour and are extended, not replaced.

## Non-goals

- No backend, gateway or proto changes in this change: no new RPC, no idempotency key, no shipping
  quote RPC, no multi-voucher or per-shop voucher contract. The shipping fee rule and the single voucher
  code stay as they are today (the voucher is previewed via `previewVoucherAction`, authoritative in
  team-promotion / team-order).
- Follow-up backend change: an idempotency key on `CreateOrder` (Open Question 3 stays open); until then the
  client-side double-submit guard is the only protection.
- No `antd` dependency.
- No routes owned by another phase: `/account/orders*` (order list/detail, phase 4), `/account/addresses`
  (phase 5), `/vouchers` (discovery), `/listing/[id]` and `AddToCartButton` (product detail) are untouched.
  This change only links to them.
- `ListingCard`, `ListingGrid`, `SearchBar`, `FilterSidebar` and `SortBar` belong to `ui-phase-discovery`;
  they are not used or changed here.
- No change to the tracking hooks: `trackEcommerce` events (`begin_checkout`, `apply_promotion`,
  `purchase`) and their payloads, `AnalyticsProvider`, `TrackLink`, `TrackImpression` and `data-*`
  attributes stay as they are.
- No new payment methods and no real payment provider; the mock simulator behaviour is unchanged.

## Dependencies

- Requires the backend change `shop-display-name` (adds a shop/seller display name to the contract). The cart
  shop header renders that name; this change cannot be completed or verified end to end until it lands.

## Impact

- `team-frontend/src/app/cart/{page,loading}.tsx`, `src/app/checkout/{layout,page,loading}.tsx`,
  `src/app/checkout/pay/[id]/{page,loading}.tsx`.
- `team-frontend/src/features/cart/*` (CartView split into server `CartGroups` plus client islands),
  `src/features/order/CheckoutView.tsx` (replaced by stepper steps), `src/features/payment/MockPaymentView.tsx`,
  `src/features/address/AddressSelectorModal.tsx` (new; `AddressModal` form reused), a new
  `VoucherModal.tsx`, `PaymentOptionsGrid.tsx`.
- `team-frontend/FEATURES.yaml`, `platform-e2e/tests/e2e/features/buyer/cart_checkout.feature` + steps and
  page objects.
- Depends on `ui-foundation` and `ui-core-components`. No dependency on other phases.
