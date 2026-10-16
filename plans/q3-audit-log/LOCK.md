# LOCK — q3-audit-log
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-10-15
spec: plans/q3-audit-log/00-spec.md (Locked 2026-10-15)
base: asdlc/q2-review-integrity (stacked on Q2 PRs agora#24/#25)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q3-audit-log-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 21db7e6e5970 | [x] |
| audit-query-filters | team-audit | [q3-audit-log-contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | b4b6c3bd0481 | [x] |
| verification-kyc-audit | team-verification | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | bc72ddc2ce03 | [x] |
| engagement-dispute-audit | team-engagement | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d0b4c078ec67 | [x] |
| domain-admin-listing-audit | team-domain | [audit-query-filters] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 895758fbe538 | [x] |
| order-admin-audit | team-order | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 88fef3a42255 | [x] |
| payment-admin-refund-audit | team-payment | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 570fdaefb137 | [x] |
| promotion-admin-audit | team-promotion | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | d66e7dc5752d | [x] |
| frontend-audit-viewer | team-frontend | [audit-query-filters] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 62c49cc6ba40 | [x] |

Round notes: round 1 failures were domain (helper names, @destructive) and order (helper names, free-text carrier, untestable audit-down ordering). Domain round 2 failed on the `status` metadata format (enum name vs stored lowercase); fixed in round 3 → pass. Engagement/domain needed extra title-prefix passes for cross-change uniqueness (text only).

## Build order (cross_change_check)
1. q3-audit-log-contract (contract, alone)
2. audit-query-filters (writes the shared e2e helper `platform-e2e/tests/e2e/support/audit_support.py`)
3. Parallel, WIP limit 3: domain-admin-listing-audit, engagement-dispute-audit, frontend-audit-viewer, order-admin-audit, payment-admin-refund-audit, promotion-admin-audit, verification-kyc-audit — disjoint repos; each writes its own feature/step files and imports audit_support.py read-only.
Hot files: root docker-compose.services.yaml and all audit.proto copies (10) — contract slice only.

## Cross-change check
pass (`cross_change_check: OK`). Earlier duplicate scenario titles were removed by per-slice prefixes ("KYC audit:", "Dispute audit:", "Listing audit:", "Order audit:", "Payment audit:", "Promotion audit:").

## Open issues / NEEDS_HUMAN
**Human clarifications Round 1 (2026-10-15)**, recorded in each affected design.md:
- Q1 Admin path is uniform: every call to a catalogue RPC by a caller holding `admin` (team-order: `order.admin`) is audited, including an admin who owns the resource. Overrides the 00-spec catalogue wording "not the owner / not the seller / for another seller". Promotion therefore has no team-domain owner lookup.
- Q2 Audit viewer dates: calendar days in UTC+07:00, `to` inclusive; times shown as `YYYY-MM-DD HH:mm:ss` UTC+07:00.
- Q3 Vietnamese copy for /admin/audit accepted as proposed.
- Q4 Every ResolveDispute status change is audited, including `investigating`.

**Orchestrator decisions (no policy change)**
- actor_id/target_type stay plain proto3 strings (not `optional` as the Locked snippet shows); empty = unset. Avoids buf breaking.
- Metadata refinements: dispute adds `status`; listing `status` = lowercase stored form; order `carrier` truncated to 64 runes (no allow-list, existing e2e sends "SPX Express"); flash sale records `listing_id` (no seller field); ad campaign records `seller_id` (stored value = the admin) + `listing_id` + `budget`; payment `amount` in minor units.
- audit-query-filters adds migration 002 index (action, created_at DESC); prod should create it CONCURRENTLY (runbook).
- E2E seeding of audit events is SQL-only (docker exec psql); producer slices create real events through their RPCs.
- ForceFailSaga gains a GetOrder load (unknown id → NotFound instead of Internal).

**Risk routing**
- auth: every producer slice (service principal + audit.write; admin-path gating) → auth-scope-reviewer.
- payment: payment-admin-refund-audit → NEEDS_HUMAN at release.
- proto: q3-audit-log-contract → contract-boundary-reviewer.
- pii-light: `kyc.review` metadata holds the subject `user_id`; viewer puts actor/target ids in the URL → privacy-reviewer at the Feature gate.

**Accepted residual risks**
- Audit row written but the change then fails (invalid transition, duplicate voucher code, refund over remainder) — the row records the attempt. Retries can write a second row (no idempotency key).
- Audit outage blocks all admin actions in the 6 producers; non-admin paths unaffected.
- `UPSTREAM_AUDIT_ADDR` must be set per environment (gitops) for verification, domain, order, payment, promotion, engagement — release item.

**Tooling gaps**: decide.py ENGINE_DOWN (no units/engine) — reviewers used their checklists.

## Re-lock 2026-10-15 (converge)
Digests updated after: tasks.md ticked in all 9 changes; Clarifications Round 2 approved by Bùi Đăng Phúc — scenario renames "Audit query: In-memory repository matches Postgres", "Payment audit: Audit is called as the service principal" (cross-initiative title clash with engagement-review-integrity) and order "Shipment on a Cancelled order keeps the audit row" (Pending is shippable by COD).
