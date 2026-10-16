# 00-spec — Review integrity

initiative: q2-review-integrity

Source: backlog Q2 (`plans/pm-review-2026-10/BACKLOG.md`), evidence `plans/pm-review-2026-10/trust-ops.md` #9.

## Goal
Every new review is tied to the buyer's own completed order, with at most one review per (order, listing). Listing and shop stars are computed only from verified, non-hidden reviews. An admin can hide an abusive review from the PDP with a reason, and every hide is recorded in team-audit. A review can no longer silently degrade to "unverified" when team-order is down.

## User stories
1. **Buyer reviews a received item.**
   - Given the buyer's order is `ORDER_STATUS_COMPLETED` and contains the listing.
   - When they submit "Đánh giá" from `/account/orders` (rating 1–5).
   - Then the review is stored with `verified_purchase=true`, shows "✓ Đã mua hàng" on the PDP, and counts in the stars.
   - Edge cases:
     - A second review for the same (order, listing) is refused with `AlreadyExists`, and the UI says "Bạn đã đánh giá sản phẩm này cho đơn hàng này."
     - An order that is not completed, belongs to someone else, or doesn't contain the listing is refused with `FailedPrecondition`.
2. **No review without an order.**
   - Given a buyer on a PDP.
   - Then there is no "Viết đánh giá" button. A hint links to `/account/orders`.
   - Edge case: CreateReview with an empty `order_id` returns `InvalidArgument`.
3. **team-order unavailable.**
   - Given team-order errors, times out, or `UPSTREAM_ORDER_ADDR` is unset.
   - When CreateReview is called.
   - Then it returns `Unavailable`, no review row is written, and no `ReviewCreated` event is emitted.
4. **Stars from verified reviews only.**
   - Given a listing with 2 verified reviews (5★, 3★) and 1 legacy unverified review (1★).
   - Then `GetListingRatingSummary` returns average 4.0 and review_count 2, and the breakdown counts only those 2.
   - `ListReviews` still shows all 3. The legacy review carries the "Chưa xác minh mua hàng" badge.
   - The same rule applies to `GetShopRatingSummary`.
5. **Admin hides a review.**
   - Given an admin on a PDP.
   - When they click "Ẩn" on a review and enter a reason (1–500 chars).
   - Then the review disappears from `ListReviews` and from both summaries for everyone. team-audit holds one event: action `review.hide`, target_type `review`, target_id the review id, actor_id the admin's user id, and metadata `{reason, listing_id}`.
   - Edge cases:
     - A non-admin caller gets `PermissionDenied` at both the gateway and engagement.
     - An unknown id returns `NotFound`.
     - An empty or oversized reason returns `InvalidArgument`.
     - Hiding a review that is already hidden returns OK and writes no second audit event.
     - team-audit unavailable → `Unavailable`, and the review stays visible (fail closed).

## Non-goals
- Profanity or PII filter via team-ai: deferred to Q23 (human decision, 2026-10-14).
- Unhide, seller reply, the review report/flag pipeline (Q23) and an admin review queue page.
- A `ReviewHidden` Kafka event. team-analytics keeps counting hidden reviews from `ReviewCreated` until Q23 or Q3. This is a known limitation, noted in the engagement proposal.
- Backfilling or re-verifying legacy reviews against team-order.
- A per-order "already reviewed" state on `/account/orders`: the `AlreadyExists` message is enough for now.
- Editing or deleting one's own review.

## Architecture decisions
- **"Delivered" means `ORDER_STATUS_COMPLETED`.** Why: the proto comment says "Buyer received & completed order", and `upstream.OrderClient.VerifyPurchase` already requires it.
- **CreateReview fails closed.**
  - Verifier errors map to `Unavailable`. A nil verifier (no `UPSTREAM_ORDER_ADDR`) also maps to `Unavailable`. Why: degrading to unverified is the loophole Q2 closes.
  - An ineligible order maps to `FailedPrecondition`, with one fixed message for all three reasons. Why: don't leak whether another buyer's order exists.
  - Every review created from now on is verified, so `verified_purchase=false` only ever marks a legacy row.
- **Uniqueness.**
  - A partial unique index on `reviews (order_id, listing_id) WHERE verified_purchase`.
  - The migration first demotes duplicate verified rows: it keeps the earliest per (order_id, listing_id) and sets `verified_purchase=false` on the rest.
  - The repository maps a unique violation to `ErrDuplicateReview`, and the handler maps that to `AlreadyExists`.
  - Why: the index enforces the rule under concurrency, and the migration cannot fail on existing data.
- **Hiding is a soft hide.** Columns `hidden_at TIMESTAMPTZ NULL`, `hidden_by VARCHAR(64)`, `hide_reason TEXT`. Hidden rows are excluded from `ListReviews` (for every caller, admins included) and from both summaries. Why: it's reversible later without a schema change, and the evidence is kept.
- **Audit write order: audit first, then hide.**
  - engagement calls team-audit `WriteAuditEvent` as service principal `service-team-engagement` with scope `audit.write`, and passes the admin's id as `actor_id`.
  - If audit fails, it returns `Unavailable` and does not hide.
  - If the hide update fails after a successful audit write, it returns `Internal`. The retry is idempotent, so the trail may show one extra attempt and never a hide without an audit event.
  - Why: no DB transaction is held across an RPC. Mirrors the `team-engagement/internal/upstream/order.go` service-principal pattern.
  - New config `UPSTREAM_AUDIT_ADDR`: when unset, HideReview returns `Unavailable`.
- **HideReview is admin-only, enforced twice.**
  - At the gateway, `adminProcedures` in `team-gateway/internal/edge/policy.go`.
  - In engagement, `RequireAdmin`, the same check `ResolveDispute` uses.
- **Summary semantics.** `review_count` in both summaries = verified, non-hidden reviews (the ones that make the average). `ListReviews.page.total` = all non-hidden reviews. The frontend labels the stars "từ N đánh giá đã mua hàng".
- **No new dependencies.**

## Contracts
```proto
// platform/engagement/v1/engagement.proto — add to EngagementService (Reviews & Ratings)
rpc HideReview(HideReviewRequest) returns (HideReviewResponse);

message HideReviewRequest {
  string review_id = 1;
  string reason = 2;      // required, 1-500 chars after trim
}
message HideReviewResponse {}

// CreateReviewRequest: field comments only (wire unchanged)
//   string order_id = 4;  // required: a COMPLETED order of the caller containing listing_id
// Review.verified_purchase = 11 comment: "true for every review created after q2; false = legacy, excluded from ratings"
// GetListingRatingSummaryResponse / GetShopRatingSummaryResponse.review_count comment: "verified, non-hidden reviews"
```
The proto is copied into `team-engagement/proto`, `team-gateway/proto` and `team-frontend/proto` (the same practice as Q1).

Existing contracts consumed unchanged:
- `OrderService.GetOrder`, consumed by engagement as a service principal.
- `AuditService.WriteAuditEvent`, consumed by engagement as a service principal with `audit.write`.
- `engagement.events/ReviewCreated`, which is now emitted only for verified reviews.

## Capabilities → slices
| capability (user-observable) | slice id (= OpenSpec change) | owner (FEATURES.yaml repo) | depends_on | uses / provides | repos touched |
|---|---|---|---|---|---|
| HideReview RPC + review rule comments | q2-review-integrity-contract | platform-core | – | provides: EngagementService.HideReview | platform-core (+ proto copies in team-engagement, team-gateway, team-frontend) |
| Verified-only reviews, one per (order, listing), fail closed, verified-only stars, audited admin hide | engagement-review-integrity | team-engagement | q2-review-integrity-contract | uses: EngagementService.HideReview, OrderService.GetOrder, AuditService.WriteAuditEvent | team-engagement, team-gateway (forwarder + admin policy), platform-e2e |
| Review entry only from orders, unverified badge, "verified" star label, admin "Ẩn" with reason | frontend-review-integrity | team-frontend | engagement-review-integrity | uses: EngagementService.HideReview, EngagementService.CreateReview | team-frontend, platform-e2e |

Hot files edited by more than one slice (the chain is sequential anyway):
- The proto copies in `team-engagement/proto`, `team-gateway/proto` and `team-frontend/proto`: contract slice only. Consumers regenerate and never edit them.
- Existing e2e review features in `platform-e2e/tests/e2e/features/{engagement,buyer,frontend}/*review*`, plus any scenario that creates a review without an order. These break under the new rules. engagement-review-integrity updates the API-level ones (`engagement/*`, `buyer/review_ratings_filter.feature`), using a shared "completed order" fixture in `platform-e2e/tests/e2e/support/`. frontend-review-integrity updates the UI-level ones (`frontend/*`). The fixture file is written by engagement-review-integrity only.

## Out of initiative
- `platform-core/docs/MASTER_CAPABILITY_MATRIX.md:20` claims "verified star reviews": a doc-drift fix that goes in Q38.

## NFR and data (initiative level; each slice's proposal.md refines them)
- **NFR:**
  - CreateReview's added latency is bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s); there is no retry.
  - HideReview p95 < 300 ms with audit up (non-gating).
  - The summary queries keep using `idx_reviews_listing_id` / `idx_reviews_seller_id`.
- **Personal data:**
  - `hidden_by` (admin user id) and `hide_reason` (free text written by staff). Purpose: moderation accountability. Basis: legitimate interest / legal obligation (Decree 85/2021 complaint handling). Retention: lifetime of the review.
  - The audit event copies the actor id and reason. Retention follows the team-audit policy.
  - There is no new buyer PII. `order_id` is already stored.
- **Risk routing expected:**
  - auth: HideReview admin gate, service principal scopes.
  - proto: the contract slice.
  - pii-light: `hide_reason` free text.

## Conventions
- **Engagement.**
  - Handler tests go in `team-engagement/internal/handler/*_test.go`. Mirror `authz_test.go` for the admin and anonymous matrix.
  - Repository tests: `pgtest_review_test.go` for Postgres, with the in-memory repo kept in parity.
  - Migration number `0011_review_integrity`.
- **Gateway.** Forwarder method in `team-gateway/internal/edge/engagement.go`, policy entry in `policy.go`, and a test next to the existing policy tests.
- **Audit client.** New `team-engagement/internal/upstream/audit.go` with its own service-principal interceptor (scopes `audit.write`). Never forward the user's principal to team-audit.
- **Frontend.**
  - `team-frontend/src/features/review/*`, `src/lib/gateway/reviews.ts`, and `src/features/order/OrderActions.tsx` (which already passes `orderId`).
  - Admin detection: `principal.scopes.includes("admin")`, the same as `app/(shop)/admin/cockpit/page.tsx`.
  - Vietnamese UI copy as written in User stories.
- **Do not touch:** team-order, team-audit (consumed as-is), team-analytics.

## Locked
2026-10-15 — Confirmed by Bùi Đăng Phúc. Verified-only reviews (COMPLETED order, one per order+listing, fail closed); stars from verified non-hidden only; admin HideReview with reason, audit-first via team-audit service principal; legacy unverified kept with badge; migration demotes duplicate verified rows; no ReviewHidden event; profanity/PII filter deferred to Q23.
