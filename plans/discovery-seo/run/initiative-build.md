# initiative-build — discovery-seo
root: /Users/phuc.buidang/Documents/agora-discovery-seo (feat/discovery-seo @ 8f9be679)
integration: .claude/worktrees/discovery-seo (asdlc/discovery-seo from 8f9be679)
wip_limit: 3
| wave | slices | base | status |
|---|---|---|---|
| 0 | discovery-seo-contract | asdlc/discovery-seo | merged |
| 1 | analytics-listing-status-export, engagement-listing-rating-events, order-listing-sales, frontend-seo | – | waiting |
| 2 | recsys-candidate-attributes, search-attribution | – | waiting |
| 3 | recsys-cart-cross-sell, search-bestseller-rating | – | waiting |
| 4 | frontend-discovery-sort-rating | – | waiting |
Per-slice state: plans/discovery-seo/run/<slice>.md

## Carry-forward
- search-attribution (auth-scope-reviewer, contract T2): team-analytics `GetSearchPerformance` handler must start with `requireAdmin(ctx)` (as handler.go:435), handler test for anonymous/buyer/admin, window 0→24 (not the 168 default of GetRecommendationPerformance), >168 InvalidArgument, aggregate rows only.
- Known base failures (out of scope): platform-core order/v1 TestCheckoutContract_PreviewCheckoutExistsOnOrderService (18 vs 17 RPCs); audit/v1 4 tests; team-gateway TestSpecCitationsResolveToExistingGatewayTests (my-vouchers cites promotion_test.go).
- order-listing-sales: spec has no VERIFIED BY lines for 13 unit-only scenarios; spec_sync --strict will flag them at converge. Adding the lines is a traceability-only spec edit → needs human re-lock of the digest (NEEDS_HUMAN at step 5). Proposed test names are in team-order/FEATURES.yaml notes.
- Go DB tests use testcontainers: run with `DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1` (red_first too), else they skip and look green.
- analytics A1: refreshed stale team-analytics/proto/platform/listing/v1/listing.proto (approved write-set widening).
- team-engagement README migrations table lacks a 0014 row and the 0010 row says 'outbox' (pre-existing doc gap, out of scope).
- frontend-seo: Dockerfile has no NEXT_PUBLIC_SITE_URL build arg (README Known gap); fine locally (localhost default), needed per env later.
- analytics listing-replay: loop exits only on seeing offset end-1; would hang if listing.events became compacted/transactional (not today). Follow-up: add stall timeout.

## 2026-11-07 progress (wave 3/4)
- recsys-cart-cross-sell: C2 approved-pending-review (red_first PASS). C3 committed 6c7e2265 (tests) + 16997ad3 (feat); red_first ts: crossSell.test.tsx and recommendations.test.ts OK; page.test.tsx green at base, accepted (mock of CartCrossSellRow only, fixture-style change). C1 tests committed 473052b3 (red: ImportError Filters); implementer running. Write-set note: C1 tests live at team-ai/tests/unit/test_*.py and tests/fixtures/recommendation_golden.json, named in tasks.md 1.1 but outside the packet write-set; accepted.
- frontend-discovery-sort-rating: e2e red committed 687722ff. e2e-author edited discovery_ui.feature and d1_discovery_steps.py (outside packet write-set) because the old "no Bán Chạy" assertion would go red after implementation; accepted. Open: spec_sync --strict flags 11 planned entries until converge flips them; two scenarios may be green at base (unsupported minRating ignored, unknown sort falls back), recorded as not red-first by nature. test-author running.
- recsys-cart-cross-sell MERGED. C1 (473052b3 tests, feat next) red_first OK on 6 new files; test_service/test_factory fixture-only green at base, accepted. Reviewers C1/C2/C3 approve. Audit: only test/e2e files outside packet (tasks.md 1.1 names them), accepted. docs-sync: gateway README:37 already states the cap; nothing else lists it.
  NEEDS_HUMAN: home_feed declares no exclude_seed, so with Q32 config now honoured a seed listing can appear in home_feed (per spec "Seed eligible when exclude_seed is off"); add exclude_seed to home_feed if the old behaviour is wanted. Plain-string popular entries in Redis are now not visible (fail closed); producers must write object entries with visible.
- frontend-discovery-sort-rating MERGED. Tests 4d8497b6, feat next; red_first PASS (8 files). Reviewer approve. Audit outside packet: discovery_ui.feature + d1_discovery_steps.py (old "no Bán Chạy" assertion), src/lib/gateway/search.ts(+test) (minRating must reach the request; tasks 1.3 implies it); accepted. Stubs in worktree regenerated locally (gitignored) instead of a symlink into the main checkout.
  NEEDS_HUMAN (spec-clarify): spec says active sort aria-current="true"; code/tests keep "page" (correct ARIA for nav links). Amend spec text.

## Converge (2026-11-07)
- features-check pass; openspec validate --strict pass x10; cross_change_check OK. spec_sync --strict: only `planned` entries pending the e2e flip.
- Fix-forward 3746b2ec: package-lock integrity for re-vendored fe-platform-api-client.tgz in team-frontend/-admin/-seller (contract slice missed it; docker npm ci failed EINTEGRITY).
- Local CI (HEAD 3746b2ec): specs/features/repo/proto/tests-asdlc pass; Go tdd 44/44 OK; Go tests pass except known base failures; team-ai 1084 + eval pass; recsys make pass; featurestore tests pass (2 pre-existing ruff FURB167); 3 frontends + fe-platform npm run check pass. team-search TestIT_* skipped (no throwaway OpenSearch).
  tdd gate reds for the final PR (human, commit shape): fixture-only tests green at base without Red-First-Exempt trailer (team-ai test_factory/test_service, admin referrerMetadata.test.ts, frontend cart page.test.tsx, admin-isolation.test.ts); e2e commit-order: consumer proto copy commit 307bdde4 counted as impl before e2e commit (gate only treats platform-core/packages/proto as contract).
  prettier vs biome disagree on union-type layout in url.ts + 3 metadata tests; biome (npm run check) is the gate, prettier left as is.
  trivy HIGH CVE-2026-78661 golang.org/x/net <0.62 in 16 go.mod: pre-existing, not touched here.
- e2e run 1 on env discovery-seo: 100 pass / 87 fail / 1 error (54 min). Triage: ENV (no recsys serving generation, modelserve/search-tombstones overlays missing, team-payment absent, FSM_REDIS_PORT/SELLER_BASE_URL/ADMIN_BASE_URL, Kafka ::1), CODE (indexer DLQ topic missing -> crash loop; toWireHit no SellerId; og:type product rejected by Next; FilterSidebar rating keys; TrackImpression not reset on new impressionId), TEST (seo_steps reason code/suspend until/svg titles/User-Agent casing, /dev/ui, SEO-36 race, SEO-2 canonical, search_sort_steps title, discovery_steps price combobox + aria-current). Decisions C17-C20 in 00-spec. Spec digests for search-bestseller-rating (C17) and frontend-seo (C18/C19) need human re-lock at step 5.
- Converge round 2 (2026-11-08): fixes 4b841a95 (DLQ topics, seller id, og:type, rating keys, impression reset) and 8f48dcd3 (hybrid hits keep sold/rating/seller, rerank only on reorder); e2e fixes 1818c7e8, 893d3190, 54dbfa9d; spec text 01f22d0a, b6873f41. e2e run 2: 129/70, run 3: 161/38, run 4 (failing modules only, Kafka advertised 127.0.0.1 by hand in ~/.cache/agora-env/discovery-seo/override.yaml): 115 pass / 17 fail.
  Remaining 17: cart_cross_sell API x10 (answers come back but never match; candidates `cx-*-pc0` with score 30/100 look like popular/category, not the e2e seeded similar set — check whether the seeded serving generation overwrites the e2e fixture), cart_cross_sell_ui x3 (row locator not visible), srm_frontend rating x2 (locator/timeout), order_listing_sales redelivered KeyError 'sold_count' (test), search_attribution paging impression id (TrackImpression or test mark).
  Known separate-PR items: seo21 robots site URL, seo39/40 admin URL.
- PAUSED 2026-11-08 at user request (machine overloaded). Env `discovery-seo` stopped with `agora_env.py dc discovery-seo stop` (containers and volumes kept). Resume: `dc discovery-seo start` (the redpanda advertise edit lives in the override file and survives), then triage the 17.
- Converge rounds 3-4 (2026-11-08, light runs, only needed services): C21 batch neighbours 9657334e; unknown context ids skipped 66d37d08; e2e test fixes 488d6a0f, c6601cf6, efdd43fb; C22 spec/test 33f331ac. All 17 remaining failures green: cart_cross_sell 12/12, cart_cross_sell_ui 3/3, order_listing_sales 10/10, srm_frontend rating scenarios, search_attribution paging. Not re-run this round (need modelserve/team-analytics, passed in run 3): hybrid/rerank attribution x9, admin search performance x2. Known separate-PR: seo21 robots site URL, seo39/40 admin URL.
- Env discovery-seo stopped again. Next: flip planned FEATURES entries to automated + spec_sync --strict, docs-sync main..asdlc/discovery-seo, feature-reviewer per change, re-lock digests changed by C17-C22 (search-bestseller-rating, frontend-seo, frontend-discovery-sort-rating, recsys-cart-cross-sell), then step 5 STOP.

## Step 5 STOP (2026-11-08)
- FEATURES flipped 48e7a91e (128 entries); spec VERIFIED BY + analytics titles 59fb2b81-ish; docs-sync READMEs + DATA_ARCHITECTURE.
- Gates: openspec validate --strict 10/10, spec_lint 10/10, spec_sync --strict 9/10 (frontend-seo: SEO-21/39/40 planned, C20), features-check pass (1861/1873), cross_change_check OK.
- Feature-reviewer (ENGINE_DOWN, LLM fallback): PASS 9/10 after round 5 fixes; frontend-seo WOULD_BLOCK(3) = SEO-21/39/40 (C20 tooling PR).
  - engagement WB1 backfill avg double rounding: fixed 30f633aa (test 93be8bfa).
  - order 5 s latency now asserted in e2e: 10/10 pass.
  - search-bestseller-rating TestIT_*: 63 pass on throwaway OpenSearch 2.19.5.
- Re-lock needed (digest changed by C17-C22 / VERIFIED BY lines): analytics-listing-status-export ecbb2c76e415, order-listing-sales 132117593c62, frontend-seo 2cb9c859f01b, recsys-cart-cross-sell cb63ad00ea0e, search-attribution 0e19d6ed8291, search-bestseller-rating 65b0d2fc16ad, frontend-discovery-sort-rating 922361ff120f. Unchanged: discovery-seo-contract, engagement-listing-rating-events, recsys-candidate-attributes.
- Bookkeeping left: tasks.md checkboxes not ticked in the slice changes.
- Env discovery-seo stopped; nothing pushed; branch asdlc/discovery-seo in .claude/worktrees/discovery-seo.

## 2026-11-09 follow-up
- C20 tooling on `chore/env-tooling-c20` (worktree `../agora-env-tooling`), merged into `asdlc/discovery-seo` (ec146e6d).
- seo.feature 27/27 on env discovery-seo; SEO-21/39/40 automated; spec_sync --strict frontend-seo passes.
- SEO-36 (frontend-seo 3.4): 5000 listings seeded in 6.3s; cold `/sitemap.xml` 4.71s (target 2s, not a gate); 100 SearchListings requests at page_size 50 not observable outside the gateway.
- tasks.md ticked; measurement-only tasks left open: analytics 2.2a, order 1.7, recsys-candidate-attributes 3.3, cart-cross-sell 4.3, search-attribution 2.0, search-bestseller-rating 3.2; order 2.3 covered by TestMigration0023_* only.
- Red-First-Exempt trailers for 3 frontend guard tests (e2a746fe).
