## Why

initiative: monetization

Seller plans become paid, termed and capped (slices promotion-plan-billing and listing-plan-cap). The seller
plan page `/seller/plans` still shows a permanent free "Đăng ký", has no price term, no period end, no way to
stop auto-renew, and the listing form has no answer when `CreateListing` is refused for the plan cap. Plan prices
were stored 100x too high (19 900 000 shown as ₫19.900.000); once team-promotion corrects them to integer VND
(199 000 / 499 000) the page must show them per 30-day term and charge-aware wording. This slice is the seller
frontend for those contracts.

## What Changes

- `/seller/plans` (team-frontend-seller): reads `GetMySubscription` (current tier, `current_period_end`,
  `auto_renew`) next to `ListPlans` and `GetEntitlements`; shows each plan's price per `term_days`, the current
  plan with its period end and renewal state, and the caps (`max_listings`, `featured_listings`) from entitlements.
- Subscribe: a confirm dialog states the price, that it is debited from the withdrawable wallet balance, that an
  upgrade takes effect now with no proration and a new 30-day period, and that a downgrade takes effect at period
  end. An insufficient-balance refusal (FAILED_PRECONDITION) is shown as a message with a link to `/seller/wallet`.
- Cancel: a `Hủy gia hạn` action (confirm dialog) via `CancelSubscription`, shown only while a paid plan is ACTIVE
  with `auto_renew` true; afterwards the page says the plan reverts to `Miễn phí` on the period end date.
- Listing cap CTA: when `CreateListing` returns RESOURCE_EXHAUSTED with reason `PLAN_LISTING_LIMIT`, the listing form
  shows the cap message and an upgrade link to `/seller/plans`; the typed form values are kept.
- Gateway module `src/lib/gateway/promotion.ts` gains `getMySubscription` and `cancelSubscription` view mappers
  (hot file shared with seller-ads-ui, which only adds ad functions).
- Modifies the `ui-seller` requirement "Wallet, plans, ads and bundles use Server Actions with the shared mutation
  contract" for its plans clause only (current plan disabled button stays; non-current buttons depend on tier order).
- No new RPC and no proto change (owned by monetization-contract); no backend change; no new route.

## Capabilities

### New Capabilities
- `seller-plan-ui`: paid-plan wording, period and renewal display, cancel, and the listing-cap upgrade CTA.

### Modified Capabilities
- `ui-seller`: the plans clause of the shared mutation-contract requirement (see delta spec).

## Impact

- team-frontend-seller: `src/app/seller/plans/page.tsx`, `src/features/seller/SubscribeButton.tsx`, new
  `CancelSubscriptionButton.tsx`, `src/features/seller/actions.ts`, `src/lib/gateway/promotion.ts`,
  `src/features/listing/actions.ts` and `ListingForm.tsx`.
- platform-e2e: new `seller/seller_plans.feature`, one scenario in `seller/create-listing.feature`; the existing
  `frontend/ui_seller_orders_money.feature::Current plan cannot be re-subscribed` is updated for the new wording
  (done by the e2e-author).
- Depends on promotion-plan-billing (GetMySubscription, CancelSubscription, term, charge) and listing-plan-cap
  (`PLAN_LISTING_LIMIT`).

## NFR

- Latency: the plans page adds one parallel unary call (`GetMySubscription`); server render p95 stays <= 300 ms
  locally with the three reads in `Promise.all`.
- Availability: if `GetMySubscription` fails the page still renders the plan list with an error Alert and all
  subscribe/cancel buttons disabled; it never shows a plan as current from a guess. Writes are never retried
  automatically (a charge must not double fire); the server idempotency key is the server's concern.
- Money: integer VND via `formatPrice`, no division or fractional display; a price below 1 000 is a defect.
- Accessibility: dialogs have a title and focus trap (existing `Modal`), buttons expose `aria-disabled` when disabled.

## Data classification

No new personal data stored or logged by this slice. Displayed: the seller's own plan tier, period end and
auto-renew flag (purpose: billing self-service; basis: contract performance; retention: owned by team-promotion,
unchanged here). The frontend keeps no copy: values render from the live read per request, nothing in
localStorage or cookies. Error strings shown to the seller never include seller ids or ledger ids.

## Threat model

- Assets: the seller's wallet balance (charged on subscribe) and the plan state.
- Abuse cases: (1) double click or replayed submit charging twice: the button is disabled while pending and the
  server charge is keyed by period (promotion-plan-billing); (2) forging another seller's subscription: the
  actions send no seller id, `GetMySubscription` / `CancelSubscription` act on the caller's own id from the
  bearer; (3) a seller without the `listing.write` scope: the page guard renders nothing and the actions fail at
  the gateway; (4) a stale page letting a seller cancel twice: the second cancel returns the unchanged state and the
  UI shows the current one after revalidation; (5) showing a paid tier from client state: the tier is only read from
  the server.
- Authz rule: both new calls require an authenticated seller principal; the frontend adds no authz of its own beyond
  the existing `listing.write` page guard.
