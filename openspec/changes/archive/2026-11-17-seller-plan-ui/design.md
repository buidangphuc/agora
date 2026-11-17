## Context

Today `/seller/plans` calls `listPlans()` and `getEntitlements(me.id)` and compares `plan.tier === entitlements.tier`.
`subscribeAction(planId)` calls `Subscribe` and revalidates. `SellState` of the listing actions carries only strings.
The proto adds `Plan.term_days`, `SellerSubscription.current_period_end` / `auto_renew`, `GetMySubscription`,
`CancelSubscription`. `GetMySubscription` returns no subscription for a FREE seller.

## Decisions

- Current plan comes from `GetMySubscription` (absent subscription means FREE), not from entitlements; entitlements
  supply only the cap values (`limits.max_listings`, `limits.featured_listings`). Why: the subscription row is the
  source of truth for period and renewal, and a single read cannot disagree with itself.
- Tier order is derived from `PlanTier` enum order (FREE < PRO < PREMIUM). A plan above the current tier is an
  upgrade (button `Nâng cấp`), below is a downgrade (`Hạ gói`). Both call `Subscribe(plan_id)`; the server decides
  immediate vs scheduled. The confirm dialog wording is static per direction.
- After any write the Server Action revalidates `/seller/plans` and the page re-reads; the UI never patches local
  state from the write response.
- The cap CTA: `saveListingAction` maps `ConnectError` code RESOURCE_EXHAUSTED whose `ErrorInfo.reason` is
  `PLAN_LISTING_LIMIT` to `SellState` with a new optional field `upgradeUrl: "/seller/plans"`; `ListingForm` renders
  the Alert plus a `LinkButton`. Other RESOURCE_EXHAUSTED reasons keep the generic error.
- Dates render in `Asia/Ho_Chi_Minh` as `dd/MM/yyyy` from `current_period_end`.

## Risks / Trade-offs

- The proto has no field for a scheduled downgrade, so the page cannot show "downgrade pending" after reload; only
  the cancel state (`auto_renew=false`) is visible. Accepted for v1 and listed as an open issue.
- The reason is read from error details; this relies on the gateway passing `ErrorInfo` through unchanged.
