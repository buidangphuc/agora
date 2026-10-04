## 1. Code — team-frontend: shared pieces and server-side logic

- [x] 1.1 Extract `computeShippingFee` to pure `src/features/order/shipping.ts` with Vitest cases (free >= 500000, HCM/HN 20000, other 35000); verify the tests pass and output is identical to today's rule
- [x] 1.2 Add checkout search-param parser/validator `src/features/order/checkoutParams.ts` (`step`, `addr`, `pay`, `voucher`; unknown step -> address; earliest-incomplete-step redirect target); verify the Vitest file covers the skip-ahead and stale-address cases
- [x] 1.3 Migrate every action in `features/cart/actions.ts` (including `addToCartAction`, also used by the PDP; address actions are owned by ui-phase-account) and `features/order/actions.ts` to the shared `ActionResult<T>` from `src/lib/action-result.ts` (ui-foundation), updating `actions.test.ts`; verify `npx vitest run src/features/cart src/features/order src/features/address`
- [x] 1.4 Add `groupByShop(items)` pure helper (group by `sellerId`, stable order, per-shop subtotal); verify its test with two sellers

## 2. Code — team-frontend: /cart

- [ ] 2.1 Rewrite `app/cart/page.tsx` as a server page rendering `Breadcrumb`, `CartGroups` (server: shop `Card`, header with the real shop display name from `shop-display-name` and the "Shop #<6 chars>" fallback for an empty name (depends on that change), rows with `Image` `aspect-square`, `PriceTag`), `OrderSummary`, and `Empty` for an empty cart; keep `getCart()` and `isCheckoutEnabled()` in parallel; verify `/cart` has no `"use client"` page and the empty, two-shop, real-name and empty-name-fallback component tests pass
- [ ] 2.2 Add islands `CartQuantityControl` (`QuantityPicker` min 1 + remove), `ClearCartButton` ("Xóa tất cả" label preserved) using the Server Actions with pending, disabled and success/error toasts and no client cart copy; verify the pending, failed-update and min-quantity scenario tests pass
- [ ] 2.3 Add `VoucherModal` (code `FormItem` + available vouchers `Radio` + `Empty`), wired to `previewVoucherAction`, preserving the `apply_promotion` payload and `data-testid="voucher-discount"`; verify the valid, invalid and focus-trap tests pass
- [ ] 2.4 Disabled "Mua hàng" with explanatory `Alert` when the kill-switch is off; mobile sticky bottom bar; verify the kill-switch test and a 375px render test (no horizontal overflow)
- [ ] 2.5 Add `app/cart/loading.tsx` (Skeleton shop card + 3 rows + summary) and `app/cart/error.tsx` (`Alert` + "Thử lại"); verify the loading footprint test and error retry test
- [ ] 2.6 Remove the old client `CartView` state copy; verify no remaining imports and `npx tsc --noEmit` passes

## 3. Code — team-frontend: checkout shell and wizard

- [ ] 3.1 Add the checkout shell (`app/checkout/layout.tsx` or `(checkout)` route group) with minimal header, `Stepper` and secure footer, hiding mega search and bottom nav while keeping `AnalyticsProvider` and `ToastProvider`; verify the shell render test and that `/cart` still renders the consumer shell
- [ ] 3.2 Rewrite `app/checkout/page.tsx` as a server page: keep `/login` redirect, kill-switch `Result`, empty-cart redirect to `/cart`; parse and validate searchParams; render the active step panel; verify redirect and skip-ahead tests
- [ ] 3.3 Address step: `Descriptions` card + `AddressSelectorModal` (`Radio` cards, "Mặc định" `Tag`, reuse `AddressModal` form, `Empty` when none, Next disabled); verify the change-address, add-address and no-address tests
- [ ] 3.4 Shipping step: single pre-selected standard option with `PriceTag` or "Freeship" `Tag` from `shipping.ts`; verify the free-shipping test
- [ ] 3.5 Payment step: `PaymentOptionsGrid` (`Radio` cards, keyboard arrows, default COD, `?pay=`), emoji icons replaced by token icon slots; verify the one-of-many and keyboard tests
- [ ] 3.6 `OrderSummary` (`Card` + `Descriptions` + `PriceTag`) with fixed rows, `data-testid="voucher-discount"` and `order-total`, desktop sticky column, mobile bottom bar + `Drawer`; verify the total-math and mobile-drawer tests
- [ ] 3.7 Confirm step with `PlaceOrderButton`: synchronous in-flight `useRef` lock, `Button isLoading disabled`, form `onSubmit` through the same guard, locked after `ok:true`, released after failure, Back/Stepper disabled while pending; verify the double-click, Enter, pending, locked-after-success and re-enabled-after-failure tests (mock `checkoutAction`, assert exactly one call)
- [ ] 3.8 Saga failure `Alert` slot with "Thử lại" and "Quay lại giỏ hàng", kept above actions without moving the CTA; verify the out-of-stock test
- [ ] 3.9 `BeginCheckoutBeacon` island firing `begin_checkout` once per entry, `purchase` after `ok:true` with the unchanged payload, `apply_promotion` unchanged; verify the tracking tests (one `begin_checkout` across four steps, one `purchase` after a double click)
- [ ] 3.10 Add `app/checkout/loading.tsx` and `app/checkout/error.tsx`; remove the old `CheckoutView.tsx`; verify `npx tsc --noEmit` passes and no `text-[` arbitrary values remain in the files of this change

## 4. Code — team-frontend: /checkout/pay/[id]

- [ ] 4.1 Rewrite `MockPaymentView` into server `Descriptions` summary + client `PaymentSimulator` (mutually exclusive, pending-aware buttons, toasts via `processMockPaymentAction`); verify the pending and exclusivity tests
- [ ] 4.2 Render `Result` states: success (links to `/account/orders` and `/`), error (Thử lại / Đổi phương thức), cancelled-by-saga (`Result` + `Alert`), keep `/login` redirect; verify one test per state
- [ ] 4.3 Add `app/checkout/pay/[id]/loading.tsx` and `not-found.tsx` (`Result status="404"`); verify the unknown-id test
- [ ] 4.4 Run the token lint, `npm run check` and `npx next build`; verify all pass and the CLS scenarios hold on a local run

## 5. E2E — platform-e2e

- [ ] 5.1 Add `team-frontend/FEATURES.yaml` entries (`status: planned`) for the new scenarios: `cart.grouped-by-shop`, `cart.voucher-modal`, `checkout.step-form`, `checkout.address-selector`, `checkout.payment-options`, `checkout.double-submit-guard`, `checkout.saga-failure-alert`, `payment.result-outcomes`, `cart.mobile-layout`; keep `cart.item-management` and extend its acceptance only if the Xóa tất cả flow changes; verify `make -C platform-e2e features-check`
- [ ] 5.2 Add `platform-e2e/tests/e2e/features/buyer/cart_checkout.feature` extending (not duplicating) `buyer/cart_management.feature`, `buyer/purchase.feature`, `promo/vouchers.feature` (voucher redemption), `payment/mock_pay.feature` and `order/saga_compensation.feature`; scenarios: two-shop grouping with the real shop name in each header, clear cart empty state, step navigation with URL `step=`, change address in modal, payment method keyboard selection, double-click Place order creates exactly one order, saga failure shows Alert with recovery, payment success/fail `Result`, 375px no horizontal overflow; verify the feature collects with `pytest --collect-only`
- [ ] 5.3 Add steps and page objects (`CartPage`, `CheckoutPage`, `PaymentResultPage`) using role/testid selectors (`voucher-discount`, `order-total`, `role:button=Xóa tất cả`, `role:button=Đặt hàng`); verify the scenarios run green against the local stack and flip the entries to `status: automated` with `covered_by`
- [ ] 5.4 Add an e2e assertion that analytics events `begin_checkout` (once), `apply_promotion` and `purchase` (once) are still emitted on the purchase path; verify green
- [ ] 5.5 Run `make -C platform-e2e features-check` then `openspec validate ui-phase-cart-checkout --strict`; verify both pass
