## Why

Plan P10 step G: a grouped live e2e regression of the three Next apps (buyer :3000, seller :3001, admin :3002) on a minimal local stack, after the F layout changes. The run found two app bugs and several test-side breaks caused by the app split.

## What Changes

- Buyer `PlaceOrderForm` and seller `ListingForm`: the submit Button sits in a fixed-width wrapper and fills it, so the platform loading spinner no longer widens the button while pending.
- Seller and admin login pages pass the app home (`/seller`, `ADMIN_HOME`) as the default `returnUrl`. Without it the login action redirected to `/`, whose page redirects again, and the client failed with "An unexpected response was received from the server".
- platform-e2e: `wait_streamed()` waits until React has swapped every streamed Suspense segment (`div[hidden][id^="S:"]`) before strict locators run. It is called from `BasePage.navigate`, the seller `go()` helper, the seller login helper and the users flash step.
- platform-e2e: checkout `voucher_discount` uses the last match, the quick actions locator finds the card that holds the links, the plans grid selector skips the equal-height wrapper, and the user-enforcement seller login uses the seller host for `/seller` paths.

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend/src/features/order/PlaceOrderForm.tsx`, `team-frontend-seller/src/features/listing/ListingForm.tsx` (+ test), `team-frontend-seller/src/app/(auth)/login/page.tsx` (+ test), `team-frontend-admin/src/app/login/page.tsx`.
- `platform-e2e/src/core/base_page.py`, `src/pages/{checkout_page,seller_listings_page}.py`, `tests/e2e/support/apps.py`, `tests/e2e/step_definitions/{d2_seller_steps,user_enforcement_steps}.py`.

## Non-goals

Backend changes, server action logic, the platform Button spinner (fixed per call site here), the GTM/warehouse environment, and the restricted-seller scenario wording (recorded in design.md).
