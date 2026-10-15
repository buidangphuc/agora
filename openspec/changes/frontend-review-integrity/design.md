# Design

## Clarifications
### Round 1 — 2026-10-15
- Q1 [data] legacy unverified review e2e: how is a `verified_purchase=false` row seeded now that the API cannot create one? → **engagement-review-integrity provides `seed_legacy_unverified_review(...)` in `platform-e2e/tests/e2e/support/review_order_support.py` via direct SQL insert into the e2e engagement DB; the frontend e2e scenario "Legacy unverified review is listed but not counted" is kept** (human)
- Q2 [boundary] pagination e2e needs >100 reviews on one listing → **seed verified rows by direct SQL (`seed_verified_reviews_sql`, same support file), not 105 real orders** (human)
- Q3 [scope] `platform-core/tools/{seed_full_marketplace.py, seed/01-base-info.sh, seed-full-marketplace.sh, run_live_user_journeys.py}` create order-less reviews → **fixed inside engagement-review-integrity (new task): create the review after completing an order, or skip the review step when that is not possible** (human)
- Q4 [outcome] Vietnamese UI copy proposed by the frontend author (FailedPrecondition, Unavailable, empty order id, hide PermissionDenied/NotFound/Unavailable, hide success, reason length, hint) → **accepted as proposed** (human)
