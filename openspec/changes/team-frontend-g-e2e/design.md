## Decisions

- Button width: the platform Button renders its spinner before the label, so a loading button grows. A `w-44` wrapper with `fullWidth` keeps the width at the two call sites the e2e checks; a platform fix (overlaid spinner) would need a revendor of all three apps and is left as a follow-up.
- Login default returnUrl: fixed in the login pages, not in `loginAction`, so the server action is unchanged.
- Streamed segments: in the seller and admin apps React briefly keeps both the hydrated boundary and the hidden streamed copy (about 200 ms), so strict Playwright locators see two matches. The test waits for the hidden segments to go instead of loosening every locator.

## Run record (2026-10-30, mainx containers via docker start)

Services: postgres, redis, redpanda, flipt, identity, domain, ai, order, payment, promotion, audit, gateway. Apps built with `NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080` and served with `next start`.

- Buyer purchase (`test_d2_checkout_order`, `test_cart_checkout`): pass except `purchase fires once` (needs team-analytics warehouse) and `apply_promotion` dataLayer (needs a GTM build arg), both environment-only and the second also failing on the baseline.
- Seller (`test_d2_seller_listing`, `test_d2_seller_orders_money`, `test_d2_seller_workplace`): 19 of 19 non-destructive pass.
- Admin (`test_admin_listing_moderation_ui`, `test_user_enforcement_ui`, `test_cockpit_health_radar`, `test_admin_isolation`): pass except the PDP goto timeout in "admin approves" under load (passes alone) and the restricted-seller scenario (see below).
- E1c: a platform voucher created through the admin UI applied at buyer checkout (discount 20,000).
- E4: UI login on each app sets only that app's cookies (`session`, `seller_session`, `admin_session`), and the other two apps still send the browser to their own login.

## Open

- `Users UI: Restricted seller sees the notice in the browser` expects the browser to stay on `/seller/new`; the seller app now redirects to `/forbidden`, which shows the same notice. The scenario wording needs a decision.
