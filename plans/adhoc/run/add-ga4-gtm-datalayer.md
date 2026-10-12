# feature-run add-ga4-gtm-datalayer
initiative: adhoc | base: feat/ga4-analytics | worktree: .claude/worktrees/add-ga4-gtm-datalayer | branch: asdlc/add-ga4-gtm-datalayer | started: 2026-10-11
## Spec gate  round1=fail(tasks write-sets) round2=pass validate=pass spec_lint=pass buf=pass decide=skipped (no units; LLM checklist)
## Preflight   locked by human 2026-10-11 (retention 13 months) validate=pass spec_lint=pass spec_sync=1 issue (e2e task lacks capability ref; expected pre-e2e) worktree=created spec-commit=a5ed0dad
## E2E         author=done run=red as expected (3/5); no-GTM red wrong reason (running frontend image built with NEXT_PUBLIC_GTM_ID=GTM-E2E0001, repo default is empty -> rebuild at converge); no-PII green = invariant guard with non-vacuity check (accepted) files=4
## Tasks
| task | scenarios | status | rounds | T0 | red_first | routed | task-reviewer | specialists |
|---|---|---|---|---|---|---|---|---|
## Open (human): none (retention resolved: 13 months raw, enforcement follow-up)
| fe-datalayer | 10 | approved r2 (privacy: allowlist+scrub; impression properties no longer in dataLayer — no GTM consumer yet) | 2 | 6cea91a4 | PASS (exempt: characterisation, code pre-existed) | – | approve (minor: SSR dataLayer test weak) | – |
| fe-callsites | 3 | approved r5 (group id per list instance; schema.ts touched type-only); r4 list ids at 7 callers, search once + query kept; minors: item_list_name dropped on search, SearchImpressions.tsx dead) | 5 | 21520fee | PASS (exempt: characterisation) | – | approve r2 (note for privacy: properties spread into dataLayer unscrubbed) | – |
| gw-boundary | 4 | approved | 1 | 0c3c6b0e | MANUAL (generated stubs): 202 at base -> 413 at head | – | approve (minors: anon assert accepts UNSPECIFIED; dup publisher) | – |
| wh-duckdb | 1 | approved | 1 | bad4598e | PASS (exempt) | – | approve | – |
| funnel-distinct | 2 | approved | 1 | 91737c76 | MANUAL: red_first.sh temp worktrees lack gitignored generated/ stubs (Go build fails at head); author saw red purchases=4 want 2, green at HEAD | – | approve (minor: reflection in test now unneeded) | – |
| wh-bigquery | 3 | approved | 1 | 8a4b67e1 | MANUAL (generated stubs): undefined seam -> green | – | approve (minors: view best-effort unlogged; identifiers unescaped, operator config) | – |
| wh-export | 2 | approved | 1 | 8ceb52b3 | exempt | – | approve (minor: atomicity weakly guarded) | – |
| recsys-columns | 4 | approved | 1 | 29dfe91c | PASS per-repo venv (fs OK; recsys EXEMPT) | – | approve (minor: dedupe per (buyer,listing,txn) — multi-item order counts each listing once) | – |
| revenue-guard | 2 | approved | 1 | 41c14666 | exempt (guard) | – | approve (minor: memory case weak) | – |
## Converge (round 1)  images rebuilt from worktree (gateway, analytics, frontend w/ harness GTM-E2E0001); e2e ga4_datalayer: 4 failed — items/one-request/group-id fail at call sites (fe-callsites reopened); no-GTM fails by harness config (human decision)
## Converge (round 2)  frontend rebuilt with coalescer; e2e 3/5 pass. fanout: one group id + distinct ids/positions OK, but beacons lack itemListId (home FeedBlock and 6 other ListingGrid callers pass no placementId; fix is outside fe-callsites write-set; fe-callsites at round 3) -> ESCALATED. no-GTM: harness sets GTM-E2E0001 -> human decision.
## Escalations
- fe-callsites: itemListId missing for ListingGrid callers without placementId (FeedBlock, RecentlyViewedRow, favorites, ListingFeed, SearchBlocks, ShopStorefrontView?, StudentRentalView?). Options: widen write-set via spec-clarify | accept (list id falls back to path) | drop "same item list id" clause.
- e2e no-GTM-when-empty vs harness GTM-E2E0001: (a) flip to "GTM loads when ID set" (b) separate empty-ID build (c) drop from e2e.
- recsys-columns: dedupe key per (buyer, listing, transaction) vs spec "at most one purchase per transaction_id".
- Human (2026-10-11): fe-callsites round 5 authorized — group id per list instance (ListingGrid mount), may touch team-frontend/src/lib/analytics/dispatcher.ts for an optional provided group id. e2e converge r3: 4/5 pass; fanout split into 2 groups by the 300 ms window.
## Converge (round 4)  FeedBlock id home_latest (collided with RecommendationsRow home_feed); e2e ga4_datalayer 5/5 PASS. Regression subset (255 tracking/recs/search tests, -n 4) running; FEATURES flip running.
## Gates       spec_sync --strict=PASS (5 e2e + 27 unit-verified) features-check=PASS e2e(ga4)=5/5 privacy=NEEDS_HUMAN (2 BLOCKING: dataLayer spreads properties unallowlisted; search_term/page_path/referrer unscrubbed in dataLayer; GTM ID not set in any prod/staging config in repo — only e2e harness) feature-reviewer=running regression=running ci_local=running
- GTM mock (e2e route) + "No external request to Google in e2e" scenario added; e2e ga4 6/6. Regression subset: 220 pass, 4 fail (diagnosis running).
## Gates (final)  e2e ga4 6/6; regression subset 224 pass / 1 fail (pre-existing roc push-list, base ec773a79); spec_sync --strict PASS; features-check PASS; feature-reviewer pass; privacy re-fixed (allowlist+scrub, GTM mock-only); ci_local specs/features/repo/proto/tests-asdlc/security PASS, tdd FAIL (red_first lacks generated stubs — separate PR); go test gateway+analytics ok; featurestore 59 pass + ruff; frontend vitest 1263 + prettier
## STOP — human approval
