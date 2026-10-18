## Why

The PDP still offers "Viết đánh giá" to any signed-in buyer, and `WriteReviewButton` opens `ReviewModal` without an `orderId`, so a review can be written with no purchase. Initiative q2-review-integrity makes engagement refuse that (`InvalidArgument` on empty `order_id`, `FailedPrecondition` for an ineligible order, `AlreadyExists` for a second review of the same order and listing, `Unavailable` when team-order is down) and excludes unverified and hidden reviews from the stars. The UI must follow: review entry only from `/account/orders`, honest labels for verified and legacy reviews, readable errors, and an admin-only "Ẩn" action. Today `createReviewAction` passes the raw gateway `err.message` to the user, so the new codes would surface as English or technical text.

initiative: q2-review-integrity

depends_on: engagement-review-integrity (backend rules, `HideReview` behind the gateway admin policy, and the shared completed-order e2e fixture).

## What Changes

- `WriteReviewButton` is removed from `ReviewSection` (header) and from the `ReviewList` empty state. In its place a hint, "Để đánh giá, hãy vào Đơn hàng của tôi và chọn "Đánh giá" ở sản phẩm bạn đã nhận.", with a link to `/account/orders`. Shown to every viewer (guest links go through the existing login guard). `ReviewModal` stays, used only by `OrderActions.ReviewButton`; its `orderId` prop becomes required.
- `ReviewList`: a review with `verifiedPurchase=false` shows the badge "Chưa xác minh mua hàng" (`data-testid="unverified-purchase"`, neutral tag) next to the existing "✓ Đã mua hàng" for verified ones.
- `ReviewSection` summary label becomes "từ {N} đánh giá đã mua hàng" (it was "{N} đánh giá"). The numbers still come from `GetListingRatingSummary`, which engagement now computes from verified, non-hidden reviews only. The star filter counts in `ReviewRatingFilter` use the same breakdown; the list under it can contain more rows (legacy ones), which is expected.
- `createReviewAction` maps `ConnectError` codes to Vietnamese text (table in the spec) instead of returning `err.message`.
- New `hideReview(reviewId, reason)` in `src/lib/gateway/reviews.ts` calling `EngagementService.HideReview`, and `hideReviewAction(reviewId, listingId, reason)` in `src/features/review/actions.ts` that validates the reason (1 to 500 characters after trim), calls the gateway, maps errors, and `revalidatePath("/listing/<id>")` on success.
- New client leaf `ReviewHideButton` (button "Ẩn", `data-testid="hide-review"`) opening a dialog "Ẩn đánh giá" with a reason textarea (1 to 500 chars, live counter), "Hủy" and "Ẩn đánh giá" buttons, inline error `Alert`. Rendered inside `ReviewItem` only when `isAdmin` is true. `listing/[id]/page.tsx` computes `isAdmin = principal?.scopes.includes("admin")` on the server (same check as `admin/cockpit/page.tsx`) and passes it through `ReviewSection` to `ReviewList`. Non-admin render output contains no hide button and no hide dialog code path in the HTML.
- The server action does not trust the client: it never receives or checks an `isAdmin` flag. The gateway enforces `admin`; the action surfaces `PermissionDenied` as "Bạn không có quyền ẩn đánh giá."
- UI e2e features that wrote reviews without an order are moved onto the completed-order fixture (see Uses and tasks): `product_detail_ui.feature` (3 seeding Givens and the submit scenario), `rvp_reviews_pagination_ui.feature`, `uif_product_detail.feature`. New e2e scenarios cover the hint, badges, label, errors and the admin hide.

No proto, gateway or engagement change in this slice. No new dependency.

## NFR

- No added network call on PDP render: `isAdmin` is decoded from the session token already read for `loggedIn`. The hide dialog is a client leaf that adds no request until submit.
- `hideReviewAction` makes one gateway call; the p95 budget is the engagement one (300 ms, non-gating). The submit button is `isLoading` and controls are disabled while pending, so a double click sends one request.
- After a successful hide the PDP is absent on the first re-render after the action resolves, with no manual reload (revalidate); no client-side cache of the removed review remains.
- Review list and summary keep the existing Suspense streaming and skeletons; failure of the review fetch still degrades to the empty state (unchanged).

## Data classification

- Hide reason (staff-written free text, 1 to 500 chars). Purpose: moderation accountability, stored and audited by the backend slice. In this slice it lives only in the dialog state and the server action call. Basis: legitimate interest / legal obligation (Decree 85/2021 complaint handling). Retention: not persisted by the frontend (no cookie, local storage or log of the text); backend retention applies. The action must not log the reason.
- Admin status (`admin` scope): used only to decide whether to render the button; not sent anywhere, not put in analytics.
- Review content and `orderId` on `ViewReview`: unchanged, already displayed or already held. `orderId` is not rendered.
- No new buyer PII, no new cookies, no new analytics fields.

## Uses

- `EngagementService.HideReview` (new, provided by q2-review-integrity-contract; request carries the review id and the reason, the exact field names are taken from the regenerated proto copy in `team-frontend/proto`).
- `EngagementService.CreateReview`, `ListReviews`, `GetListingRatingSummary` (existing; semantics changed by engagement-review-integrity).
- The shared completed-order e2e fixture in `platform-e2e/tests/e2e/support/`, written by engagement-review-integrity only; this slice imports it and never edits that file.

## Threat model

- Assets: review integrity (a hidden review must really disappear for everyone), admin capability, the reason text.
- Abuse cases:
  1. A non-admin forges a call to `hideReviewAction` (server actions are public POST endpoints). The gateway policy answers `PermissionDenied`; the action returns the Vietnamese message and nothing changes. Rendering the button only for admins is cosmetic.
  2. A client sends an oversize or empty reason. The action rejects it before the gateway call; engagement also answers `InvalidArgument`.
  3. A stale admin token (scope revoked) still sees the button until re-login. The gateway is authoritative, so the click fails with `PermissionDenied`.
  4. A buyer tries to review from the PDP by calling `createReviewAction` with an empty `orderId`. The action rejects it with a Vietnamese message without a gateway call; engagement also answers `InvalidArgument`.
  5. Reason text rendered back into the page. It is never rendered (hidden reviews disappear), and any error text is plain React text, no HTML injection.
  6. Hidden-review leakage to admins: the list shows nothing for hidden rows even for admins, so the UI has no unhide path (non-goal).
- Authz rule: show the button when `principal.scopes.includes("admin")`; the gateway and engagement decide. The action trusts no client-supplied role.
