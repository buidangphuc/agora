# initiative-build — discovery-followups
root: /Users/phuc.buidang/Documents/agora-discovery-followups (feat/discovery-followups)
integration: .claude/worktrees/discovery-followups (asdlc/discovery-followups)
wip_limit: 3
| wave | slices | base | status |
|---|---|---|---|
| 0 | discovery-followups-contract | asdlc/discovery-followups | merged |
| 1 | analytics-listing-deleted-status, order-cod-sales-count, search-listing-stock-lookup | asdlc/discovery-followups @ bc90c721 | merged |
| 2 | recs-in-stock-filter | asdlc/discovery-followups @ 10d70496 | merged |
Per-slice state: plans/discovery-followups/run/<slice>.md

## Carry-forward
- search-listing-stock-lookup (auth-scope NIT, contract T1): once team-gateway regenerates Connect stubs, `/platform.search.v1.SearchService/GetListingStock` is registered via the embedded Unimplemented handler and answers 501, not 404. Nothing is forwarded upstream. The e2e "gateway does not expose the lookup" must assert the real status; if it is 501 and the spec says 404 → spec-clarify (human).
- Worktrees have no fe-platform/node_modules or platform-e2e/.venv: use VENV=/Users/phuc.buidang/Documents/agora/platform-e2e/.venv/bin/ and symlink node_modules temporarily.
- Go DB tests: DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1.
- order-cod-sales-count (e2e-author): 9 unit-only scenarios lack `**VERIFIED BY**` lines in the spec, so spec_sync --strict flags them. Adding them is a traceability-only spec edit and changes the digest → NEEDS_HUMAN re-lock at step 5 (same as order-listing-sales in discovery-seo). Proposed names: TestCodSales_PendingToCompletedIsRefused, _NotCountedBeforeCompletion, _OutboxFailureRollsBackCompletion, _SeveralListingsEmitOneEventEach, _ReturnRequestDoesNotChangeCount, BenchmarkCodCompletionLatency, TestNoRPCAcceptsSoldCount, TestMigration0024_{HistoricalCodCompletedOrdersAreCountedOnce,IgnoresLedgerAndNonCodOrders,BackfillTwiceDoesNotDuplicate,NoCodOrders}. test-author must use these names.
- order-cod-sales-count: e2e reads soldCount from SearchListings; "exactly one ListingSalesChanged within 5 s" is asserted at unit level only. "Completing an online-paid order does not count it again" is likely green at base (guards a no-op).
- search-listing-stock-lookup (e2e-author): the e2e asserts gateway 404 per spec (no 501 tolerance). If the stack shows 501 → spec-clarify (human). The log scenario needs the handler to log a line with `GetListingStock` and the id count. The slow-index e2e uses `docker pause` (unbounded), so the precise 200 ms deadline is unit-only. grpcurl/ghz run as throwaway containers; `LSL_OPENSEARCH_CONTAINER` defaults to agora-opensearch-1 (unverified for the env project).
- analytics-listing-deleted-status (e2e-author): packet widened with fia_steps.py, fia_flow.py, fia_job_driver.py (the real step/flow files). The e2e adds a Qdrant check (`visible` present and false), because the spec THEN asserts only the gateway response, which is green at base. Spec amendment optional (NEEDS_HUMAN at step 5). The replay seed writes DuckDB from the featurestore image's python duckdb; Go/Python DuckDB file compatibility is unverified until the live run. Each new real-stack scenario waits up to 390 s.

## spec-clarify 2026-11-12 (Bùi Đăng Phúc)
- search-listing-stock-lookup: gateway may answer 404 or 501 unimplemented; requirement = not forwarded.
- order-cod-sales-count: add VERIFIED BY lines for unit-only scenarios (real test names).
- analytics-listing-deleted-status: e2e THEN adds item point `visible=false`; v2-lock VERIFIED BY → test_committed_lock_matches_registry.
- recs-in-stock-filter (orchestrator, same class as COD traceability): cart_cross_sell VERIFIED BY path recsys/ → recommendations/.
Spec edits land on asdlc/discovery-followups; LOCK.md digests must be re-signed for these 4 slices at step 5.
- Analytics feature-review fail: replay e2e step can pass on a no-op replay → e2e-author fix-forward on the analytics slice branch, re-merge.
- Analytics and COD merged into integration before their feature-reviewer finished (gate is advisory); feature-reviewer re-runs at converge.
- Spec edits committed ed210949 on asdlc/discovery-followups. New digests (need human re-sign in LOCK.md at step 5): order-cod-sales-count bd427fb8ebb5 (was bd2154828398), analytics-listing-deleted-status 70ba6a4972a7 (was f35440598ae1), search-listing-stock-lookup 4247f421e082 (was 4d10ed023587), recs-in-stock-filter b66568578ff2 (was 8543a46fda3c). Contract unchanged e6afe5654013.
- Fix-forwards merged: search b29ec501 (ebb1c761), analytics 366c7d85 (0f2cc3b4).
- Converge started: env discovery-followups up from integration d59b8aae (+recs-retrieve-budget overlay); proto gate: buf lint+breaking vs origin/main pass (ci_local proto fails only because buf cannot clone a ref from a worktree .git).
- e2e run 1 (2026-11-12, env discovery-followups, 8 files): 26 pass / 38 fail / 1 error in 21 min. Triage:
  - ENV (tooling gap): agora_env `up` did not include minio-init (mlflow-artifacts bucket → every recsys job failed → no serving collection → all Recommend 503) or team-payment (online-paid order flows 503). Fixed by hand: `dc.sh up -d --no-build minio-init team-payment-migrate team-payment`. agora_env should add one-shot init jobs and payment for order slices.
  - TEST: listing_stock_lookup hardcodes agora-opensearch-1; ghz image obvionaryltd/ghz does not exist; gateway no-forward log check not scoped to the gateway call. fia replay seed uses `--user nonroot` unknown in the featurestore image. Fix-forward by e2e-authors.
- e2e live per file so far: listing_stock_lookup 12/12 PASS (after fix 089c3050); fia deleted/replay 3/3 PASS (after fix b3ac5091; Python seed on Go DuckDB works); order_listing_sales all PASS (after adding team-payment); cod_sales_count 4/5 (missing Given step → e2e-author); candidate_attributes passes except the teardown restore (fresh env has no real data, structural gate refuses) → in_stock_recommendations + cart_cross_sell 14/16 fail with no serving generation (fresh env has none; mainx has). e2e-author investigating a self-sufficient bootstrap.
- Gateway on this env answers 404 (gateway image is main's :local, stubs without GetListingStock); spec accepts 404 or 501.
- e2e confirmation run 4 on integration 4748ca64: 64 passed, 0 failed, 1 teardown error (candidate_attributes restore needs real data; fresh-env only, pre-existing discovery-seo harness). Fix-forward commits: search 089c3050, analytics b3ac5091, COD 30458900, recs 4748ca64 (recs scenarios now bootstrap a fixture serving generation on a fresh env).

## Converge result (2026-11-12, asdlc/discovery-followups @ 1601d80d)
| gate | result |
|---|---|
| live e2e (8 files, env discovery-followups) | 64 pass / 0 fail; 1 teardown error (candidate_attributes restore needs real data, fresh env only) |
| features-check | pass |
| openspec validate --strict x5 | pass |
| spec_sync --strict | contract, COD, recs pass; analytics FAIL (2 spec titles ≠ feature titles); search FAIL (2 unit-only scenarios lack VERIFIED BY) |
| cross_change_check | OK |
| asdlc specs/features/repo/tests-asdlc | pass; proto: buf lint+breaking vs origin/main pass (ci_local proto only fails cloning from a worktree) |
| Go vet+test team-order/search/analytics | pass (team-search index ITs skip without OPENSEARCH_TEST_URL; ran separately against a throwaway OpenSearch) |
| team-ai pytest + eval | 1121 pass, eval 1.0 |
| featurestore / recsys | 94 / 499 pass |
| 3 frontends npm run check | pass (2224/955/877) |
| security (gitleaks, trivy, semgrep) | pass |
| docs-sync | 77f6f2b6 (team-search, team-gateway READMEs) |
| feature-reviewer x5 | pass (ENGINE_DOWN, LLM fallback) |
| privacy | contract: needs-human (stale-copy parity brings Storefront/moderation note defs into 6 consumers; defs only) |
- 2026-11-12 traceability fix 663d9df6 (human approved): spec_sync --strict passes for all 5. Digests to re-sign in LOCK.md: contract e6afe5654013 (unchanged), order-cod-sales-count bd427fb8ebb5, analytics-listing-deleted-status 41b5fbcbf2c9, search-listing-stock-lookup 7c59d3b88345, recs-in-stock-filter b66568578ff2.
