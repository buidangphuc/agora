# LOCK — discovery-seo
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-09 (re-signed; first signed 2026-11-07)
spec: plans/discovery-seo/00-spec.md (Locked 2026-11-07, Clarifications C1–C22)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| discovery-seo-contract | platform-core | – | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 9f65dc582f61 | [x] |
| analytics-listing-status-export | team-analytics | – | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | a66d42b437fa | [x] |
| engagement-listing-rating-events | team-engagement | discovery-seo-contract | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 5fc38b87d6b5 | [x] |
| order-listing-sales | team-order | discovery-seo-contract | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 488d6cd49d1f | [x] |
| frontend-seo | team-frontend | discovery-seo-contract | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | e5f948c14b9e | [x] |
| recsys-candidate-attributes | platform-recsys | analytics-listing-status-export | 2 | pass | pass (4 PERM warnings, batch job, no authz path) | pass | ENGINE_DOWN (LLM fallback) | bb20ce78ec87 | [x] |
| recsys-cart-cross-sell | team-ai | discovery-seo-contract, recsys-candidate-attributes | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 7a57e954530c | [x] |
| search-attribution | team-search | discovery-seo-contract | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 0615f0696345 | [x] |
| search-bestseller-rating | team-search | order-listing-sales, engagement-listing-rating-events | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | b2312c956a0d | [x] |
| frontend-discovery-sort-rating | team-frontend | search-bestseller-rating | 1 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 415c53ce963e | [x] |

decide.py was not run: it needs a `--units` file, and none was built. The spec-quality-reviewer checklist is the verdict.

## Build order (cross_change_check)
1. discovery-seo-contract (sequential, alone)
2. analytics-listing-status-export, engagement-listing-rating-events, order-listing-sales, frontend-seo
3. recsys-candidate-attributes, search-attribution
4. recsys-cart-cross-sell, search-bestseller-rating
5. frontend-discovery-sort-rating

Hot-file pairs that must not run in parallel:
- **`team-search/internal/rpc/search.go`:** search-attribution, then search-bestseller-rating.
- **`team-frontend/src/features/search/SearchBlocks.tsx` and `src/app/(shop)/search/page.tsx`:** search-attribution, frontend-seo and frontend-discovery-sort-rating.
- **`team-frontend/FEATURES.yaml`:** every slice touching team-frontend, serialised by validate_plan at build.

## Cross-change check
pass (`cross_change_check: OK`). Round 1 had an ownership violation (search-bestseller-rating edited team-frontend capabilities). It was fixed by splitting out frontend-discovery-sort-rating.

## Open issues / NEEDS_HUMAN
- **COD orders (C12).** Today a COD order counts toward `sold_count` only once team-order emits `OrderPaidEvent` for it. COD orders that complete without that event never count as "Bán chạy". Accept this, or follow up with a COD paid event.
- **Deleted listings (C15 / D1).** A deleted listing exports `status` null, because no `LISTING_STATUS_DELETED` enum exists. Null fails closed and the listing is hidden from recommendations. A DELETED enum is a possible proto follow-up.
- **Replay (C15).** The `listing-replay` command needs the live team-analytics consumer stopped while it runs, because DuckDB takes an exclusive lock. This is a deploy runbook step.
- **Deploy order for recsys.** The order is: analytics export and replay, then featurestore `item_attributes@v2`, then recsys publish or backfill, then team-ai (`recsys-cart-cross-sell`). If team-ai ships before `visible` exists, every placement fails closed and shows no recommendations.
- **`in_stock_only` (C14).** It stays a documented no-op, because no producer writes `in_stock`. A real stock source (C3-U1) is a follow-up.
- **Sitemap (C11).** The sitemap is cached with `revalidate=3600`. A cold render needs 100 sequential `SearchListings` calls (page size 50). The 2 s figure is a target, measured but not gating.
- **`NEXT_PUBLIC_SITE_URL`.** It is inlined at build time, so staging and prod need a per-env build arg. Envs are deferred, so this is local only.
- **Risk routing.**
  - `proto`: discovery-seo-contract.
  - `auth`: the admin-only route for `GetSearchPerformance`.
  - No payment or pii routing. New fields carry no personal data. `ListingRatingChanged` envelopes carry no principal (C3).
- **Migration numbers.** team-order 0023 and team-engagement 0015 assume the current base. Renumber them if main moves before the build.

## Re-lock 2026-11-09
Re-signed by Bùi Đăng Phúc after converge. The digests changed because of ticked tasks.md, the converge clarifications C17–C22 and aligned `VERIFIED BY` lines.
