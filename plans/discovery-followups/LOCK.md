# LOCK — discovery-followups
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-12 (re-signed 2026-11-12 in chat after spec-clarify and traceability edits)
spec: plans/discovery-followups/00-spec.md (Locked 2026-11-12, Clarified 2026-11-12)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| discovery-followups-contract | platform-core | [] | 2 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | e6afe5654013 | [x] |
| order-cod-sales-count | team-order | [] | 3 | pass | pass | fail r1, fail r2 → r3 fix verified by orchestrator | ENGINE_DOWN (LLM fallback) | bd427fb8ebb5 | [x] |
| analytics-listing-deleted-status | team-analytics | [discovery-followups-contract] | 2 | pass | pass (1 warning: PERM-scenario, internal pipeline, no caller authz) | pass (r1) | ENGINE_DOWN (LLM fallback) | 41b5fbcbf2c9 | [x] |
| search-listing-stock-lookup | team-search | [discovery-followups-contract] | 2 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | 7c59d3b88345 | [x] |
| recs-in-stock-filter | team-ai | [search-listing-stock-lookup] | 2 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | b66568578ff2 | [x] |

Round 2 fixes for the slices that passed in round 1 were not re-reviewed by spec-quality-reviewer; validate and spec_lint were re-run on all five.

## Build order (cross_change_check)
1. discovery-followups-contract (sequential, alone)
2. analytics-listing-deleted-status, order-cod-sales-count, search-listing-stock-lookup
3. recs-in-stock-filter

Hot-file pair that must not run in parallel: analytics-listing-deleted-status and recs-in-stock-filter both edit
`platform-e2e/tests/e2e/flows/item_attributes_support.py`. They are already in different waves.

## Cross-change check
pass (`cross_change_check: OK`)

## Open issues / NEEDS_HUMAN
- **Risk routing:** `proto` (contract slice). There is no auth, payment or pii routing. GetListingStock is service-only, gated by principal type `service` plus scope `search:read`.
- **Decisions made 2026-11-12** (00-spec "Clarified"):
  - In stock means projected base stock > 0.
  - A missing entry drops only that candidate.
  - team-ai looks up only the top `limit × 2` candidates, capped at 20 (at most 2 calls), and refills from the next ones.
  - COD completion p95 may rise by at most 5 ms.
  - DELETED is terminal.
- **Verified in code, not open:**
  - A counted COD order can never be cancelled, because COMPLETED and CANCELLED are terminal.
  - A COD return can never reach REFUNDED, because approval is refused.
  - So COD needs no reversal. The proposal notes that this changes if COD refunds are ever enabled.
- **Assumptions by authors.** Reject any of these at sign-off:
  - order-cod-sales-count:
    - The migration is `0024_listing_sales_cod`.
    - COD is detected by `payment_method == CodPaymentMethod (1)`.
    - The backfill event id is `hash(listing, sold_count, '0024-backfill-cod')`.
    - The new test `TestNoRPCAcceptsSoldCount` is proposed.
  - search-listing-stock-lookup:
    - The server-side index deadline is 200 ms, and a slow index returns UNAVAILABLE within 400 ms.
    - An empty id or a list of more than 10 ids returns INVALID_ARGUMENT.
    - A repeated id gets a repeated entry.
    - A tombstoned listing returns stock absent.
    - The gateway returns 404 (no route). The gateway has no GetListingStock reference today.
  - recs-in-stock-filter:
    - When the refill budget runs out, team-ai keeps the candidates already verified and does not fail closed. A call timeout during refill still fails closed.
    - The names of the env settings for the search address and the service credential are left to the implementer. team-ai has no outgoing service-principal client yet, so the pattern to mirror is team-search's ClassifyTags caller.
    - `Candidate.in_stock` becomes `bool | None`.
    - The `fallback` flag on an empty result is left to the implementer.
  - analytics-listing-deleted-status:
    - Replay can re-derive DELETED only while the delete event is still on `listing.events`. Older rows stay null or stale. This is a known gap.
    - The e2e precondition is seeded by writing a null-status row into DuckDB while team-analytics is stopped.
- **Tooling gap:** `decide.py` did not run. In round 1 it crashed with `No module named 'yaml'` because reviewers used the system python. In round 2 it needs a `--units` file. Every spec gate is an LLM-checklist verdict.
- **Deferred (local first):** none of the deploy-only items from discovery-seo are in scope.
