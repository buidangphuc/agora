## 1. Code — team-frontend: seller shell

- [x] 1.1 Add `SellerSidebar` (client: collapse toggle, `usePathname` active link with `aria-current`, `localStorage` preference), `SellerNavDrawer` (on `Drawer`, closes on navigation) and `SellerPageHeader` (server: `Breadcrumb`, title, primary action slot); verify Vitest covers collapse, active link and Drawer close
- [x] 1.2 Rebuild `src/app/seller/layout.tsx` on them (shop card with `Avatar`/`Tag` and the real shop display name from `shop-display-name`, "Shop #<6 chars>" only for an empty name; depends on that change, no emoji icon text, scope-gate `Result`, redirect to `/login` when unauthenticated); verify the 403, collapse, 375px Drawer, real-name and empty-name-fallback tests pass
- [x] 1.3 Add segment `loading.tsx`, `error.tsx` (`Result` + `reset` + link to `/seller`) and `not-found.tsx` under `src/app/seller/`; verify a test renders each

## 2. Code — team-frontend: Workplace and Table List

- [x] 2.1 Add a shared `parseListParams(searchParams)` helper (q, status, page with safe defaults) and a cursor-walk `getListingsPage(page)` over `listMyListings`; verify unit tests for invalid values and page 3
- [x] 2.2 Rebuild `/seller` page: KPI `Statistic` row from real data (no hard-coded 0; a KPI whose source call fails is hidden), quick actions `Card`, recent-orders `Table`; verify the KPI derivation test passes and a test asserts a failed source hides its cell and no hard-coded metric value remains
- [x] 2.3 Add `SellerFilterBar` (client, debounced `router.replace`, page reset) and the product `Table` + `Pagination` + `Image` thumbnails (1:1, lazy after first screen) + `Empty`/`Alert` states; verify URL, empty and invalid-param tests pass
- [x] 2.4 Replace `DeleteListingButton` with `DeleteListingModal` (confirm `Modal`, pending, `Alert`, toast, `revalidatePath("/seller")`); verify confirm, pending and failure tests pass and no `confirm(` remains
- [x] 2.5 Add `loading.tsx` for `/seller` with a skeleton of the same footprint; verify the skeleton and page KPI row heights match in a test

## 3. Code — team-frontend: listing studio

- [x] 3.1 Convert `ListingForm` fields to controlled state grouped in `Card` sections with `FormItem`, `Input`, `Select`, `Radio`; keep hidden `imageKeys`/`variants` and `saveListingAction`; verify existing create/edit tests still pass
- [x] 3.2 Add field validation (title, price > 0, stock >= 0, category) on blur/submit plus server `SellState` mapping to fields, pending submit (`isLoading`, disabled sections), success toast/`Result`, error `Alert` + toast; verify the required-field and pending tests pass
- [x] 3.3 Rebuild Magic Listing as a controlled suggestion `Card` with per-field "Áp dụng", no `document.getElementById`, no default title, `Alert` + retry on failure; verify non-destructive apply and failure tests pass
- [x] 3.4 Update `/seller/new` and `/seller/[id]/edit` pages (header, 403 `Result`, `not-found`), sticky action bar on mobile, `loading.tsx` skeleton; verify the 403 and 375px sticky bar tests pass

## 4. Code — team-frontend: orders and order detail

- [ ] 4.1 Rebuild `/seller/orders`: server page reading `?status=&q=&page=`, link `Tabs` with counts, `Table`, `Pagination`, row actions (Chi tiết, Xác nhận gửi) via a small client `ShipOrderButton` island; delete `useState` filtering from `SellerOrdersList`; verify the tab-in-URL reload test passes
- [ ] 4.2 Make `updateOrderStatusAction` return `{ ok, error?, data? }` and `revalidatePath("/seller/orders")` and `/seller/orders/[id]`; verify the action tests pass
- [ ] 4.3 Rebuild `/seller/orders/[id]` as a server Advanced Profile on `getOrder` + `getShipmentTracking` (header `Descriptions`, status `Statistic`, `Stepper`, `Tabs` items/shipment with `Table`/`Timeline`, recipient `Descriptions`), ship confirm `Modal`, `PrintButton` + print stylesheet, `not-found` for unknown/not-owned; verify tests for real data, ship pending/failure and not-found pass
- [ ] 4.4 Add `loading.tsx` for both order routes; verify skeleton tests

## 5. Code — team-frontend: analytics, wallet, plans, ads, bundles

- [ ] 5.1 Rebuild `/seller/analytics` (`Statistic` row, funnel `Progress` rows, revenue `Table`, `?range` `Tabs`, `Alert`/`Empty`); delete `SellerAnalyticsMock`; verify range and failure tests pass and no mock import remains
- [ ] 5.2 Rebuild `/seller/wallet` (balance `Statistic`, payout confirm `Modal`, ledger `Table` + `Pagination` via `?page=`) and make `requestWalletPayoutAction` return `{ ok, error?, data? }`; verify payout pending, zero-balance disabled and revalidate tests pass
- [ ] 5.3 Rebuild `/seller/plans` (plan `Card` grid, current `Tag`) and `subscribeAction` on the shared contract; verify current-plan and toast tests pass
- [ ] 5.4 Rebuild `/seller/ads` and `/seller/bundles` forms on `FormItem`/`Select`/`Checkbox`/`Input`, tables on `Table`, actions `createAdCampaignAction` / `createBundleAction` on the shared contract with pending/disabled/toast; verify the error-field and success tests pass
- [ ] 5.5 Keep `/sell` as a server redirect to `/seller/new`; verify a test asserts the redirect

- [ ] 5.6 Add `/seller/shop` shop profile form (display name `FormItem`, `upsertStorefrontAction` returning `ActionResult`, gateway wrapper for `UpsertStorefront`) and a sidebar link; depends on `shop-display-name`; verify Vitest covers save, blank and >80-char validation, and the name shows on `/shop/<id>`

## 6. Code — team-frontend: cross-cutting

- [ ] 6.1 Replace every raw hex, `text-[..]`/`p-[..]` and sub-12px text in `src/app/seller/**`, `src/app/sell/**`, `src/features/seller/**`, `src/features/listing/ListingForm.tsx`, `src/features/order/SellerOrdersList.tsx` with tokens; verify `node scripts/check-tokens.mjs` is clean
- [ ] 6.2 Confirm no `TrackLink`, `TrackImpression`, `SearchImpressions`, `AnalyticsProvider` or tracking `data-*` was added, removed or changed (`git diff` over those files is empty) and `/listing/[id]` row links are unchanged; verify the link test passes
- [ ] 6.3 Run the gate; verify `npm run check` and `npx next build` pass

## 7. E2E — platform-e2e

- [ ] 7.1 Add `team-frontend/FEATURES.yaml` entries (`status: planned`) for: `seller.shell-navigation`, `seller.workplace-dashboard`, `seller.product-table-filters`, `seller.listing-delete-confirm`, `seller.listing-studio-validation`, `seller.magic-listing-assist`, `seller.order-detail`, `seller.analytics-range`, `seller.wallet-payout-confirm`, `seller.route-states`, `seller.responsive-375`; each `acceptance` line maps 1:1 to a spec scenario; verify `make -C platform-e2e features-check`
- [ ] 7.2 Extend `tests/e2e/features/seller/listing_delete.feature` (confirm Modal, cancel keeps row, confirm removes row), `create-listing.feature` (required-field error, pending submit, success toast), `edit-listing.feature` (foreign listing shows 403 `Result`), `seller_order_management.feature` (status tab and `status=` in URL after reload, `?page=`), `seller_analytics.feature` (range tab updates `range=`), `fulfillment_and_payout.feature` (payout confirm Modal); verify each runs green against the local stack
- [ ] 7.3 Add `tests/e2e/features/seller/seller_cockpit.feature` with steps and page objects for: sidebar collapse, 375px Drawer navigation, Workplace KPI row and quick actions, product search in the URL, order detail (real data, ship with toast), Magic Listing apply-on-request, no horizontal scroll at 375px, empty/error/not-found recovery; verify green
- [ ] 7.4 Add one tracking-regression scenario to the existing tracking feature set (impression + click with placement attribution still emitted, seller funnel counts them); verify green
- [ ] 7.5 Flip the new FEATURES.yaml entries to `automated` with `covered_by`; verify `make -C platform-e2e features-check`
- [ ] 7.6 Run `openspec validate ui-phase-seller --strict`; verify it is valid
