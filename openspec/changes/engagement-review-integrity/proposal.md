## Why

`ReviewService.CreateReview` in `team-engagement/internal/service/review.go` degrades to an unverified review whenever team-order fails, times out, or `UPSTREAM_ORDER_ADDR` is unset, and it accepts an empty `order_id`. Anyone can therefore post stars without buying, and `GetListingRatingSummary` / `GetShopRatingSummary` average every row, so unverified and abusive reviews move the stars. There is also no way to remove an abusive review. The initiative spec (`plans/q2-review-integrity/00-spec.md`, locked 2026-10-15) closes this: every new review belongs to the caller's own COMPLETED order, one per (order, listing); stars count verified, non-hidden reviews only; an admin can hide a review with a reason and every hide is audited. The proto (`EngagementService.HideReview`, field comments) is owned by `q2-review-integrity-contract`; the UI is `frontend-review-integrity`.

initiative: q2-review-integrity

## What Changes

- **team-engagement**
  - Migration `0011_review_integrity` (up and down):
    - Adds `hidden_at TIMESTAMPTZ NULL`, `hidden_by VARCHAR(64)`, `hide_reason TEXT` to `reviews`.
    - First demotes duplicate verified rows: per (`order_id`, `listing_id`) it keeps the earliest `created_at` (id as tiebreak) and sets `verified_purchase=false` on the rest.
    - Then creates the partial unique index `uq_reviews_order_listing_verified` on `(order_id, listing_id) WHERE verified_purchase`.
    - Down drops the index and the three columns (demoted rows stay demoted).
  - `CreateReview` fails closed:
    - Empty `order_id` -> `InvalidArgument`.
    - Nil verifier or any verifier error or timeout -> `Unavailable`; no row, no `ReviewCreated`.
    - Verifier says not eligible (not the buyer's, not `ORDER_STATUS_COMPLETED`, listing not in the order, or order unknown) -> `FailedPrecondition` with one fixed message.
    - Otherwise the review is stored with `verified_purchase=true` and the order's `seller_id`.
    - Unique violation -> `repository.ErrDuplicateReview` -> `AlreadyExists`.
    - `ReviewCreated` is enqueued in the same transaction as the row, so only for stored verified reviews (unchanged mechanism).
    - From now on `verified_purchase=false` marks legacy rows only.
  - Repository (Postgres and in-memory, kept in parity):
    - `GetRatingSummary` and `GetShopRatingSummary` aggregate only `verified_purchase AND hidden_at IS NULL`.
    - `ListReviews` excludes hidden rows; `page.total` counts all non-hidden rows (verified or not) matching the rating filter.
    - New `HideReview(ctx, reviewID, adminID, reason)` and `GetReview(ctx, id)` (or equivalent) with `ErrReviewNotFound`.
    - Summary queries keep using `idx_reviews_listing_id` / `idx_reviews_seller_id`.
  - New `HideReview` handler:
    - `interceptor.RequireAdmin` first (same check `ResolveDispute` uses).
    - Reason trimmed, 1-500 chars (runes) else `InvalidArgument`.
    - Empty `review_id` -> `InvalidArgument`; unknown id -> `NotFound`.
    - Already hidden -> OK, no second audit.
    - Otherwise audit first, then hide.
  - New upstream client `internal/upstream/audit.go` (`AuditClient`, config `UPSTREAM_AUDIT_ADDR`, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS`):
    - Calls `AuditService.WriteAuditEvent` as service principal `service-team-engagement`, type `service`, scope `audit.write`, with its own interceptor that replaces any outgoing principal. The user's principal is never forwarded.
    - Event: `action=review.hide`, `target_type=review`, `target_id=<review id>`, `actor_id=<admin user id>`, `metadata={reason, listing_id}`.
    - Unset address -> nil client -> `HideReview` returns `Unavailable`.
    - Audit error -> `Unavailable`, review stays visible.
    - Hide update failure after a successful audit -> `Internal`; the retry is idempotent, so the trail may show one extra attempt and never a hide without an audit event.
  - Wiring in `cmd`/`bootstrap` and config (`UPSTREAM_AUDIT_ADDR`, default empty) and the local compose file.
- **team-gateway**
  - `EngagementForwarder.HideReview` in `internal/edge/engagement.go` (write path, no retry).
  - `engagementv1connect.EngagementServiceHideReviewProcedure` added to `adminProcedures` in `internal/edge/policy.go`; the pinned-set test in `policy_test.go` and the unrouted guard are updated.
- **E2E stack requirement:** the e2e stack MUST run team-audit and set `UPSTREAM_ORDER_ADDR` and `UPSTREAM_AUDIT_ADDR` for team-engagement (`docker-compose.services.yaml`; `.env.example` and README updated).
- **platform-core tools:** `tools/seed_full_marketplace.py`, `tools/seed/01-base-info.sh`, `tools/seed-full-marketplace.sh`, `tools/run_live_user_journeys.py` create a review only after completing an order, or skip the review step.
- **platform-e2e** (owned by this slice, API level only; legacy and bulk rows are seeded by direct SQL in the fixture)
  - Shared fixture `platform-e2e/tests/e2e/support/review_order_support.py` (completed order + verified review), written by this slice only.
  - Existing API-level review features migrated to it (listed in tasks.md); new feature `engagement/review_integrity.feature`.
- **Known limitation (accepted, no new event):** there is no `ReviewHidden` event. team-analytics keeps counting a hidden review from its earlier `ReviewCreated` until Q23 or Q3 adds one. Also `ReviewCreated` carries no verified flag, and legacy unverified reviews were already counted by analytics.
- Not changed: team-order, team-audit, team-analytics, the proto (consumed from the contract slice), editing or deleting one's own review, unhide, profanity/PII filter (Q23), backfill of legacy reviews.

## NFR

- `CreateReview` adds at most one `OrderService.GetOrder` call, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s, also capped by the inbound deadline); never retried. Timeout surfaces as `Unavailable`.
- `HideReview` p95 < 300 ms with team-audit up (non-gating target, measured in the engagement handler test harness against local Postgres with a stub audit server); one audit RPC bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS`, one indexed update.
- Availability: team-order down blocks new reviews only (reads unaffected); team-audit down blocks hiding only (fail closed); both with `Unavailable`.
- Limits: reason 1-500 chars after trim; `review_id` max the existing id length; page size limits unchanged (default 20, max 100).
- Migration must succeed on existing data (duplicates demoted first) and be re-runnable by the migration tool's normal rules.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `reviews.hidden_by` (admin user id) | personal (staff identifier) | moderation accountability | legitimate interest / legal obligation (Decree 85/2021 complaint handling) | lifetime of the review row |
| `reviews.hide_reason` (free text typed by staff, <= 500 chars) | personal-light (may quote buyer content) | explain the moderation decision | same | lifetime of the review row |
| `reviews.hidden_at` | metadata | exclusion from reads and summaries | same | lifetime of the review row |
| audit event (`actor_id`, metadata `reason`, `listing_id`) | personal-light | immutable moderation trail | same | team-audit retention policy |
| `order_id`, `user_id`, review text | existing, unchanged | verified purchase proof, display | contract | unchanged |

`hide_reason` is returned by no read API and is not put in logs, spans or the outbox (it goes only to the DB row and the audit event). No new buyer PII. Nothing new goes to analytics.

## Threat model

- **Assets:** star ratings (ranking and buying trust), the review table, the audit trail, the service principal credential for team-audit.
- **Abuse cases:**
  - Fake review with no purchase: `order_id` required and verified, no unverified path.
  - Fake review by forcing team-order down or leaving `UPSTREAM_ORDER_ADDR` unset: fail closed, `Unavailable`, nothing stored.
  - Reviewing with someone else's order, a non-completed order, or a foreign listing: `FailedPrecondition` with one fixed message, so the response does not reveal whether another buyer's order exists.
  - Review spam on one order: partial unique index, enforced under concurrency (two parallel calls yield one row, one `AlreadyExists`).
  - Non-admin hiding reviews (competitor sabotage): `adminProcedures` at the gateway plus `RequireAdmin` in engagement (a call that bypasses the gateway is still denied).
  - Hiding without a trail: audit written first; audit failure means no hide. Residual: one extra audit entry if the hide update fails after the audit write.
  - Forged audit entries: the user's principal is never forwarded; only the service principal with `audit.write` writes, and `actor_id` comes from the verified Principal, never from the request.
  - Reason as a vector (stored XSS): reason is never returned by any read API; if shown later it must be rendered as text.
  - Race of two admins hiding one review: the second sees it already hidden and returns OK with no second audit (a concurrent pair may produce two audit entries; accepted).
- **Authz rule:** `HideReview` requires an authenticated caller with the `admin` scope at the gateway and in engagement. `CreateReview` requires an authenticated user; the order must belong to that user (checked against team-order's `buyer_id`).
