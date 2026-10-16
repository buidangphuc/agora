# LOCK — q2-review-integrity
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-10-15
spec: plans/q2-review-integrity/00-spec.md (Locked 2026-10-15)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q2-review-integrity-contract | platform-core | [] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 2aa48290033b | [x] |
| engagement-review-integrity | team-engagement | [q2-review-integrity-contract] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 34d2044206e1 | [x] |
| frontend-review-integrity | team-frontend | [engagement-review-integrity] | 3 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 08e2c89fcc76 | [x] |

Notes on the rounds:
- Round 3 of each slice was text-only (VERIFIED BY citations and wording) after the reviewer had already passed it in round 2 (contract: round 2 → pass; engagement: rounds 1+2; frontend: round 2 → pass).
- The round 1 fail of engagement was caused by the contract's anonymous error code; that was fixed in the contract.

## Build order (cross_change_check)
1. q2-review-integrity-contract (contract, sequential)
2. engagement-review-integrity
3. frontend-review-integrity

The whole chain is sequential. Shared files:
- `platform-e2e/tests/e2e/support/review_order_support.py`: written by engagement only, imported by frontend.
- `platform-e2e/tests/e2e/step_definitions/rvp_steps.py`: engagement switches the seeding, frontend only adjusts UI assertions.

## Cross-change check
pass (`cross_change_check: OK`). The round 1 duplicate scenario name `Reason length boundary` was renamed in frontend.

## Open issues / NEEDS_HUMAN

**Risk routing**
- auth: HideReview admin gate at both the gateway and engagement; engagement's service principal gains `audit.write` → auth-scope-reviewer.
- proto: contract slice → contract-boundary-reviewer.
- pii-light: `hidden_by` and `hide_reason`, copied into the audit event → privacy-reviewer at the Feature gate.

**Decisions locked in** (00-spec plus design.md Clarifications Round 1)
- Review creation:
  - A review needs a COMPLETED order of the caller that contains the listing; at most one verified review per (order, listing).
  - An empty order id returns InvalidArgument, an ineligible order returns FailedPrecondition with one fixed message, and a duplicate returns AlreadyExists.
  - Any upstream failure, or `UPSTREAM_ORDER_ADDR` unset, returns Unavailable; no row is written and no event is emitted.
- Ratings: stars and review_count use only verified, non-hidden reviews. `ListReviews` drops hidden reviews; its total counts all non-hidden ones.
- HideReview:
  - Anonymous callers get Unauthenticated and non-admins PermissionDenied; authz is checked before validation.
  - The reason must be 1–500 runes after trim.
  - An unknown id returns NotFound.
  - Hiding an already-hidden review returns OK with no new audit event.
  - The audit write happens first. If audit is unavailable, the call returns Unavailable and the review stays visible. If the hide fails after the audit write, the call returns Internal; a retry writes a second audit event (at-least-once).
  - MarkReviewHelpful on a hidden review returns NotFound.
- Migration 0011 keeps the earliest verified duplicate and demotes the rest, then creates a partial unique index.
- E2E seeding:
  - Legacy unverified rows and the pagination test's 105+ verified reviews are seeded by direct SQL, which needs docker access to the Postgres container (as the `oic_inv_stack` and `plp_stack` helpers do).
- Seed scripts: the 4 `platform-core/tools` scripts are fixed in the engagement slice (task 12).
- Compose: task 13 sets `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` for engagement in the root `docker-compose.services.yaml`. The human chose to fix the seed tooling inside the slice, so a stack-config edit at the root is in scope.
- UI copy: the Vietnamese texts the frontend author proposed are accepted.

**Assumptions accepted by the reviewer and flagged for the human**
- team-order NotFound maps to FailedPrecondition.
- Two admins hiding the same review at the same moment may write two audit rows.

**Known limitations** (non-goals)
- There is no `ReviewHidden` event, so team-analytics keeps counting hidden reviews.
- There is no unhide.
- The profanity/PII filter is deferred to Q23.

**Tooling gaps**
- `decide.py` returns ENGINE_DOWN (missing yaml module), so the reviewers used their own checklists. Already tracked.
