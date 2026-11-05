## Context

Full UI e2e run of main images against compose project `mainx` (buyer container :3000, seller app :3001 and admin
app :3002 on the host), one file at a time, 2026-11-05. 326 red tests were triaged: 100 TEST-DRIFT, 1 APP-BUG,
4 UNSURE, 221 ENV. This change fixes the drift and the bug, records the UNSURE root cause and lists the ENV reds.

## Root cause of most drift: React's hidden streaming segment

Next streams a page's Suspense content into `<div hidden id="S:n">` nodes and swaps each into place with an inline
script shortly after. At `domcontentloaded` the hidden copy can still be in the DOM, so a locator matches twice
(Playwright strict mode) or counts double (48 cards instead of 24). A probe on :3000 showed a single render: the
copy is gone within 250 ms. The steps read too early.

Decision: one helper. `goto_streamed(page, url, wait_until="domcontentloaded")` in `src/core/base_page.py` navigates
and then calls `wait_streamed(page)` (polls until no `div[hidden][id^="S:"]` is left, 10 s cap); `BasePage.goto`
uses it, so page objects and raw step navigations share one path. `LoginPage.login` calls `wait_streamed` before
filling, so it is safe after a guard redirect to `/login`. The README "Streamed pages" gotcha says steps never
`page.goto` and then read the page.

Alternative rejected: wrapping `page.goto` in the `world` fixture. It would also change scenarios that read the
fallback on purpose (skeleton and streaming tests) and hide the wait from readers.

## Fixes by pattern

**A. Streaming segment (68 tests)** - `goto_streamed` in: `ai_interaction_steps.py` (assistant, chat, `/seller/new`,
PDP), `checkout_coherence_ui_steps.py` (`goto_with_query`), `d1_account_steps.py` (`_goto`), `d1_discovery_steps.py`
(`_goto`), `d1_pdp_steps.py` (`_open`), `d2_orders_steps.py` (list and `open_detail`), `d2_seller_steps.py`
(skeleton reload), `engagement_steps.py` (collection page), `loop_events_steps.py` (`_goto`),
`order_lifecycle_ui_steps.py` (`goto`), `orders_ui_steps.py`, `prm_rma_ui_steps.py` (buyer returns tab),
`product_detail_steps.py` (variant URLs), `review_integrity_steps.py`, `discovery_steps.py` (paged and filtered
search), `listing_moderation_ui_steps.py` (seller list and edit), `c2_shop_steps.py` (visitor PDP); plus
`LoginPage.login`.

**B. Semantic token aliases (6)** - status tags and badges use `text-text-warning|success|danger`
(`fe-platform/packages/tokens/src/tokens.mjs` aliases of promo-dark, success-dark, danger-dark):
`d1_account_steps.py` status tags, `d2_orders_steps.py` `TONE_CLASS` and the pending saga dot.

**C. App split (7)** - `tests/e2e/support/apps.py` gains `app_base` and routes `/admin*` to `ADMIN_BASE_URL`
next to `/seller*`. `login_return_steps.py` requests and expects the guard redirect on the app that serves the path;
`referrer_hygiene_steps.py` opens `/admin/*` on the admin app; the admin app has no GTM, so the dataLayer scenario
asserts the admin app pushes nothing (step text changed, scenario title kept); the non-admin cases scenario opens
`/admin/cases` on the admin app (the buyer footer admin links were removed by design); the seller token lint runs in
`team-frontend-seller` over `src/app/seller,src/app/sell,src/features/seller`.

**D. Renamed copy or moved markup (16)** - order carries `platformVoucherCode` and `platformDiscountAmount`
(`journey_steps.py`); Radix checkbox SSR counted by `role="checkbox"` only (`cart_selection_ui_steps.py`);
"Sắp xếp theo" without a colon (`d1_discovery_steps.py`, `search_sort_steps.py`); search result cards are
the direct children of the results grid, since each card now holds inner `.grid` nodes (`d1_discovery_steps.py`
`_CARDS`); the gallery stage is compared relative to its parent, because the stage column is `md:sticky` and
clicking a thumbnail may scroll the page (`d1_pdp_steps.py`); the voucher reason is the input's
`aria-describedby` error text and the error toast behind the modal is found with CSS `[role="alert"]`, and the
shop header name is the header link (`d2_cart_steps.py`); `lg:sticky` sits on the summary
wrapper (`d2_checkout_steps.py`); toasts behind a modal are found with the CSS `[role="alert"]` (a Radix modal
`aria-hidden`s the rest of the page, so `get_by_role` cannot see them) (`d2_orders_steps.py`); order thumbnails are
1:1 and at least 64px (80px from `sm`), the detail is capped at 1024px (`d2_orders_steps.py`, spec change below);
seller thumbnail box is `w-14` (`d2_seller_steps.py`); the admin dispute detail card is the list item in `main` that shows
"Trạng thái: " (only the deep-linked detail card does), not the first list, which is now the AdminShell nav
(`dispute_cases_steps.py`); seller return rows and the payment summary ignore the hidden streaming copy
(`filter(visible=True)` in `src/pages/seller_order_returns_page.py`); colours are compared after converting
`color(srgb r g b)` to `rgb()` (Tailwind v4 serialisation) (`ui_foundation_steps.py`).

**E. Missing waits (4)** - cockpit null payload waits for the stubbed `/api/admin/metrics` response and
"Chưa có dữ liệu" (`c1_steps.py`); the search impression scrolls the grid into view and waits for the
`view_item_list` push; the filter drawer focus check targets the Radix `role="dialog"` and waits for focus
(`discovery_steps.py`).

**Pinned diff base** - `promotion_admin_vouchers_steps.py` diffed `fe-platform/docs/gaps.md` against
`origin/main`; since PR #92 merged, main no longer lists G9-G11 and the "control" assert failed forever. The base
is pinned to `05978392` (origin/main before #92, the base the change was built on). A grep of `platform-e2e/src`,
`tests` and `scripts` for `origin/main`, `git show`, `git diff` and `merge-base` found no other step that diffs
against a moving ref (`tests/e2e/static/pft_support.py` only lists and logs history).

## App bug: seller notification link on the buyer app

`team-frontend/src/features/notification/NotificationsView.tsx` rendered `link_url` as a relative `next/link`
href. team-notification emits `/seller/orders/{id}` for the new-order notification; since the seller-app split the
buyer app has no `/seller` routes, so the link was a 404. Fix: `notificationHref()` sends a path equal to `/seller`
or starting with `/seller/`, `/seller?` or `/seller#` through the existing `sellerAppUrl()` (`src/lib/apps.ts`);
every other value is unchanged, empty stays `#`. Session cookies are per host name, not per port, and
`session_cookies()` sets the seller cookie, so the seller lands signed in.

## Spec changes

- `ui-orders` "Order screens follow the type scale, colour and responsive rules": the detail is capped by the
  layout's `max-w-5xl` (Tailwind 3 default 64rem = 1024px) on purpose (archive 2026-11-02-team-frontend-layout-orders,
  design "Risks"); the spec said 960px. The spec follows the layout; `d2_orders_steps.py` asserts `<= 1024`.
- `admin-moderation-ui` "Admin entry links": the buyer `ConsumerShell` footer admin links were removed with the
  admin app (archive 2026-11-02-team-frontend-admin-app). Admin links live in the admin cockpit only; a non-admin
  opening one sees the admin layout's "Không có quyền" (`team-frontend-admin/src/app/admin/layout.tsx:18`), not
  the page-level "Cần tài khoản Quản trị".
- `order-lifecycle-ui` "Lifecycle notifications open real pages": the old text said no UI code change was needed
  beyond rendering `link_url`; it now names the seller-app rewrite.

## UNSURE: PDP of a fresh moderation listing never reaches domcontentloaded (4 tests, not fixed here)

Tests: `test_admin_cases_ui.py` dismiss, `test_admin_listing_moderation_ui.py` approve, `test_listing_report_ui.py`
guest and buyer; after the streaming fix also `test_seller_listing_status_ui.py` "seller sees a clean listing as
selling" (its strict-mode red was hiding the same timeout). All open `/listing/{id}` of a listing created by `_listing_body` (title
`Clean laptop stand modui<hex10>`, description `moderation ui e2e`, `cat-electronics`, no images).

Findings:
- `curl -w` on such a PDP: status 200, first byte in 0.05 s, body cut at about 121 KB and curl exit 18 (incomplete
  chunked encoding); a healthy PDP is about 131 KB with exit 0. Playwright waits for a document end that never
  comes, so `goto` times out at 30 s.
- `docker logs team-frontend-svc`: `Error: failed to pipe response` caused by
  `A previously unvisited boundary must have exactly one root segment. This is a bug in React.` (digest
  3493644215), from `.next/server/app/(shop)/listing/[id]/page.js`.
- It is size dependent, not data dependent: same seller, title and category, only the description length varied:
  1-5 chars ok, 7-17 chars fail, 19-200 chars ok. `cat-fashion`, no category or a longer description pass with the
  same text; images and stock do not matter. Upstream services answered normally (recommendations answer
  `unavailable` and the row renders nothing; no reviews, so the AI summary returns null).
- The HTML stops inside `#reviews` after the AI summary fallback (`B:2`, `ReviewSection.tsx` Suspense around
  `AiReviewSummary`); the review summary, the review list boundary and Q&A are never written.
- The throw is in the Fizz flush of the React that Next 15.5.28 vendors (`next/dist/compiled/react-dom/cjs/
  react-dom-server.node.production.js` 19.2.0-canary-0bdb9206-20250818): `flushSegment` outlines a completed
  boundary when `isEligibleForOutlining` (`byteSize > 500`, line 3953) and
  `flushedByteSize + boundary.byteSize > progressiveChunkSize` (12800, lines 4005, 6374-6384), and otherwise
  inlines it and throws when `completedSegments.length !== 1` (lines 6390-6398). Which side a boundary falls on
  depends on how many bytes were flushed before it, which is why a few characters of description flip the result.

Proposed fix (separate change, app code): upgrade Next to a release whose vendored React no longer throws on this
outlining path, and add a regression test that renders the PDP for description lengths around the window (a
vitest over `renderToReadableStream` of `ReviewSection` with filler, or an e2e that creates listings with
descriptions of 1-40 characters and asserts curl exit 0). Interim workaround if the upgrade waits: wrap
`AiReviewSummary` so the boundary always renders one element (for example an empty `div` instead of `null`) and
re-run the size sweep; this is a mitigation to verify, not a confirmed fix.

## Known reds: environment (221 tests, out of scope)

Grouped by the dependency the `mainx` stack lacks. The 2 git rows of `test_promotion_admin_vouchers.py` were the
`origin/main` drift above and are fixed by the pinned base.

| Missing dependency | Tests | Files (tests) |
|---|---|---|
| recsys/featurestore jobs need qdrant + featurestore Redis (FSM_REDIS 6380) + MLflow (5050) + platform-recsys | 65 | `test_fia_item_attributes.py` (3), `test_fir_recsys_item_attributes.py` (4), `test_fsd_governed_datasets.py` (2), `test_fsm_materialization.py` (3), `test_gbl_ranker_serving.py` (2), `test_mla_nearline_ctr.py` (2), `test_mlr_ml_recsys.py` (12), `test_ofb.py` (5), `test_ple_home_feed.py` (1), `test_recsys_auto_train.py` (7), `test_rgp_generation_publish.py` (7), `test_rgt_gbdt_ranker.py` (9), `test_rss_recommendation_attribution.py` (1), `test_ssa_serving_switch_atomicity.py` (7) |
| ga4-relay not recreated with ga4-relay-e2e.override.yaml (no GA4_API_SECRET, no MP mock on 18120) | 31 | `test_ga4_purchase_relay.py` (31) |
| llm-fake overlay not active | 23 | `test_apr_breaker.py` (3), `test_apr_prompt.py` (6), `test_apr_quota.py` (4), `test_apr_routing.py` (7), `test_apr_tracing.py` (1), `test_ar2_stream_lifetime.py` (2) |
| uif assert_agora guard refuses to stop containers of compose project 'mainx' (destructive scenario needs proje | 19 | `test_uif_account.py` (2), `test_uif_cart.py` (1), `test_uif_discovery.py` (3), `test_uif_foundation.py` (1), `test_uif_orders.py` (2), `test_uif_pdp.py` (5), `test_uif_seller.py` (5) |
| Recommend -> 503 'service unavailable' from team-ai (no recsys generation/qdrant/featurestore in stack | 18 | `test_ai_auth.py` (1), `test_buyer_full_funnel_journey.py` (1), `test_d1_discovery.py` (2), `test_d1_pdp.py` (1), `test_ple_placement_engine.py` (1), `test_recommendations.py` (5), `test_recsys_consent_context.py` (1), `test_rss_serving_safeguards.py` (4), `test_tpr_trained_recs_local.py` (2) |
| MLflow registry not reachable at localhost | 12 | `test_model_registry.py` (4), `test_recommendations.py` (6), `test_tpr_trained_recs_local.py` (2) |
| /dev/ui only exists under `next dev` | 12 | `test_ui_components.py` (12) |
| GA4 import fixture env missing on team-analytics (GA4_EXPORT_SOURCE=fixture GA4_EXPORT_DATASET/GA4_EXPORT_FIXT | 11 | `test_ga4_export_import.py` (10), `test_recsys_consent_context.py` (1) |
| payment-ledger.override.yaml not active on team-payment (hold window 604800s) | 10 | `test_d2_seller_orders_money.py` (1), `test_pear_authz_payout.py` (1), `test_plp_payout_holdback.py` (6), `test_plp_refund_deduction.py` (1), `test_seller_payout.py` (1) |
| modelserve router (vLLM/TEI) not running | 7 | `test_modelserve.py` (7) |
| assistant grounding needs RAG indexing (qdrant + embeddings) | 3 | `test_assistant_grounding.py` (3) |
| Prometheus/otel-collector not running (localhost | 2 | `test_cockpit_metrics.py` (1), `test_session_revocation.py` (1) |
| test shells out to git, but the run used a main export without .git | 2 | `test_promotion_admin_vouchers.py` (2) |
| seller form selects category 'cat-laptop', which the mainx data has no seed for | 2 | `test_seller.py` (2) |
| session-retention.override.yaml not active on team-identity | 2 | `test_session_retention.py` (2) |
| @hybrid needs modelserve/embedding overlay (needsModelserveOverlay) | 1 | `test_hybrid_retrieval.py` (1) |
| search-tombstones.override.yaml not active on team-search-indexer | 1 | `test_srm_deletes.py` (1) |

## Re-run results (worktree, one file at a time)

Before: main images, full run 2026-11-05. After: this worktree, `team-frontend` rebuilt with the app fix, other
images rebuilt from this worktree. Every remaining red is ENV or the UNSURE stream abort.

| File | Before (passed/failed) | After (passed/failed) | Still red |
|---|---|---|---|
| `test_admin_cases_ui.py` | 3/2 | 4/1 | UNSURE: PDP stream abort (React Fizz bug, see above) |
| `test_ai_interaction_events.py` | 4/12 | 16/0 | - |
| `test_buyer_full_funnel_journey.py` | 0/2 | 1/1 | ENV: Recommend 503 (recsys not in stack) |
| `test_c1_cockpit_real_data.py` | 4/1 | 5/0 | - |
| `test_cart_selection_ui.py` | 6/1 | 7/0 | - |
| `test_checkout_coherence_ui.py` | 6/1 | 7/0 | - |
| `test_d1_account.py` | 24/10 | 34/0 | - |
| `test_d1_discovery.py` | 19/7 | 24/2 | ENV: Recommend 503 (2 attribution tests) |
| `test_d1_pdp.py` | 22/14 | 35/1 | ENV: Recommend 503 (similar-items attribution) |
| `test_d2_cart_page.py` | 11/2 | 13/0 | - |
| `test_d2_checkout_flow.py` | 13/1 | 14/0 | - |
| `test_d2_orders_actions.py` | 3/1 | 4/0 | - |
| `test_d2_orders_detail.py` | 4/4 | 8/0 | - |
| `test_d2_orders_layout.py` | 4/4 | 8/0 | - |
| `test_d2_orders_list.py` | 5/2 | 7/0 | - |
| `test_d2_orders_returns.py` | 1/3 | 4/0 | - |
| `test_d2_seller_quality.py` | 3/3 | 6/0 | - |
| `test_discovery_tracking.py` | 2/1 | 3/0 | - |
| `test_dispute_cases_ui.py` | 2/3 | 5/0 | - |
| `test_engagement_collections.py` | 0/1 | 1/0 | - |
| `test_login_return.py` | 8/4 | 12/0 | - |
| `test_loop_events.py` | 9/3 | 12/0 | - |
| `test_order_lifecycle_ui.py` | 9/3 | 12/0 | - |
| `test_orders_ui.py` | 8/1 | 9/0 | - |
| `test_prm_rma_return_refund_ui.py` | 6/2 | 8/0 | - |
| `test_product_detail.py` | 9/1 | 10/0 | - |
| `test_promotion_admin_vouchers.py` | 51/2 | 53/0 | - |
| `test_referrer_hygiene.py` | 7/3 | 10/0 | - |
| `test_review_integrity_ui.py` | 5/2 | 7/0 | - |
| `test_search_facets.py` | 8/2 | 10/0 | - |
| `test_search_mobile.py` | 1/2 | 3/0 | - |
| `test_search_price_sort.py` | 0/1 | 1/0 | - |
| `test_seller_listing_status_ui.py` | 0/4 | 3/1 | UNSURE root cause: the clean moderation listing PDP hits the same stream abort |
| `test_shop_display_name.py` | 10/1 | 11/0 | - |
| `test_ui_foundation.py` | 2/2 | 4/0 | - |
