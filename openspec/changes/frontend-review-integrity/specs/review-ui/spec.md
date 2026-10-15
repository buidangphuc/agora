## ADDED Requirements

### Requirement: The PDP has no review entry point, only a hint to the orders page

`ReviewSection` and the empty state of `ReviewList` SHALL NOT render a "Viết đánh giá" button (`data-testid="write-review"`) for any viewer. They SHALL render a hint with a link to `/account/orders`. `WriteReviewButton` SHALL be removed. The "Đánh giá" button on a completed order item (`OrderActions` `ReviewButton`) SHALL remain the only review entry and SHALL always pass the order id to `ReviewModal`.

#### Scenario: Signed-in buyer sees the hint, not the button
- **WHEN** a signed-in buyer opens a PDP that has reviews
- **THEN** the reviews section contains zero elements with `data-testid="write-review"` and zero buttons named "Viết đánh giá"
- **AND** it contains a link to `/account/orders` inside a hint that begins "Để đánh giá, hãy vào"
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewSection.test.tsx` `renders the orders hint and no write button`

#### Scenario: The empty state shows the hint instead of the write button
- **WHEN** a PDP with zero reviews and no star filter is rendered for a signed-in buyer and for a guest
- **THEN** the "Chưa có đánh giá nào" empty state shows the same hint link to `/account/orders` in both cases and no "Viết đánh giá" button
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewList.test.tsx` `empty state links to orders for buyer and guest`

#### Scenario: Following the hint reaches the orders page
- **WHEN** a signed-in buyer clicks the hint link on a PDP
- **THEN** the browser lands on `/account/orders`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `PDP offers no write button and links to the orders page`

#### Scenario: Review from a completed order still works
- **WHEN** a buyer with a completed order containing the listing clicks "Đánh giá" on that item at `/account/orders`, picks 5 stars, types a comment and submits
- **THEN** a success toast "Đánh giá sản phẩm thành công!" shows, the modal closes, and the PDP lists the review with the "✓ Đã mua hàng" badge
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `Reviewing a completed order item shows a verified review`

#### Scenario: A guest following the hint is sent to login, not to a review form
- **WHEN** a guest clicks the hint link on a PDP
- **THEN** the guest is redirected to `/login` (the orders page guard), no review modal opens and no CreateReview call is made
- **AND** if a guest's session has expired and CreateReview is called, the gateway answers `Unauthenticated` and the action shows "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại."
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `PDP offers no write button and links to the orders page`; `team-frontend/src/features/review/actions.test.ts` `maps Unauthenticated on create`

### Requirement: Review submission errors are shown in Vietnamese

`createReviewAction` SHALL reject an empty `orderId` locally without a gateway call, and SHALL map gateway `ConnectError` codes to these messages, returning `fail(message)`: `AlreadyExists` to "Bạn đã đánh giá sản phẩm này cho đơn hàng này."; `FailedPrecondition` to "Chỉ có thể đánh giá sản phẩm trong đơn hàng đã hoàn thành của chính bạn."; `Unavailable` to "Chưa xác minh được đơn hàng của bạn. Vui lòng thử lại sau."; `InvalidArgument` and an empty `orderId` to "Vui lòng đánh giá từ trang Đơn hàng của tôi."; `Unauthenticated` to "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại."; any other error to "Gửi đánh giá thất bại.". The raw gateway message SHALL NOT be shown. On failure `ReviewModal` SHALL stay open with the message in its inline `Alert` and the form content intact.

#### Scenario: Second review for the same order and listing
- **WHEN** the gateway answers `AlreadyExists` to CreateReview
- **THEN** the action returns `{ ok: false, error: "Bạn đã đánh giá sản phẩm này cho đơn hàng này." }` and no revalidation happens
- **VERIFIED BY**: `team-frontend/src/features/review/actions.test.ts` `maps AlreadyExists`

#### Scenario: Ineligible order
- **WHEN** the gateway answers `FailedPrecondition`
- **THEN** the action returns the error "Chỉ có thể đánh giá sản phẩm trong đơn hàng đã hoàn thành của chính bạn."
- **VERIFIED BY**: `actions.test.ts` `maps FailedPrecondition`

#### Scenario: team-order unavailable
- **WHEN** the gateway answers `Unavailable`
- **THEN** the action returns the error "Chưa xác minh được đơn hàng của bạn. Vui lòng thử lại sau." and the modal keeps the typed comment
- **VERIFIED BY**: `actions.test.ts` `maps Unavailable`; `team-frontend/src/features/review/ReviewModal.test.tsx` `keeps the modal open and the comment on error`

#### Scenario: Empty order id never reaches the gateway
- **WHEN** `createReviewAction` is called with `orderId` of `undefined` or `""`
- **THEN** it returns the error "Vui lòng đánh giá từ trang Đơn hàng của tôi." and the gateway client is not called
- **VERIFIED BY**: `actions.test.ts` `rejects an empty order id locally`

#### Scenario: Unknown failures hide technical detail
- **WHEN** the gateway call throws a non-Connect error with message "ECONNRESET" or a Connect error with another code
- **THEN** the action returns the error "Gửi đánh giá thất bại." and the raw message is absent from the result
- **VERIFIED BY**: `actions.test.ts` `falls back to a generic message`

#### Scenario: Duplicate review shown end to end
- **WHEN** a buyer submits a second review for the same item of the same completed order from `/account/orders`
- **THEN** the modal shows "Bạn đã đánh giá sản phẩm này cho đơn hàng này." and stays open
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `A second review for the same order item is refused`

### Requirement: Verified and legacy reviews are labelled honestly

`ReviewItem` SHALL show "✓ Đã mua hàng" (`data-testid="verified-purchase"`) when `verifiedPurchase` is true and "Chưa xác minh mua hàng" (`data-testid="unverified-purchase"`) when it is false; exactly one of the two per review. The summary box SHALL read "từ {N} đánh giá đã mua hàng" where N is `summary.reviewCount`, and SHALL keep "Chưa có đánh giá" when N is 0.

#### Scenario: Legacy review carries the unverified badge
- **WHEN** the list renders three reviews, two verified and one with `verifiedPurchase=false`
- **THEN** two items contain "✓ Đã mua hàng", one item contains "Chưa xác minh mua hàng", and no item contains both
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewList.test.tsx` `badges verified and unverified reviews`

#### Scenario: Rating label counts verified purchases
- **WHEN** the summary has `reviewCount` 2 and `averageRating` 4.0
- **THEN** the summary box shows "4.0" and "từ 2 đánh giá đã mua hàng", and no text of the old form "2 đánh giá" without the suffix
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewSection.test.tsx` `labels the count as verified purchases`

#### Scenario: No reviews keeps the empty summary
- **WHEN** the summary has `reviewCount` 0
- **THEN** the box shows "Chưa có đánh giá" and no "từ 0 đánh giá đã mua hàng"
- **VERIFIED BY**: `ReviewSection.test.tsx` `keeps the empty summary text`

#### Scenario: Stars exclude the legacy review end to end
- **WHEN** a PDP has 2 verified reviews (5 and 3 stars) and 1 legacy unverified 1-star review
- **THEN** the summary shows "4.0" and "từ 2 đánh giá đã mua hàng", the list shows 3 reviews, and exactly one carries "Chưa xác minh mua hàng"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `Legacy unverified review is listed but not counted`

### Requirement: Admins can hide a review with a reason; others cannot see the control

`listing/[id]/page.tsx` SHALL compute `isAdmin` as `principal.scopes.includes("admin")` on the server and pass it to `ReviewSection` and `ReviewList`. `ReviewItem` SHALL render the "Ẩn" button (`data-testid="hide-review"`) only when `isAdmin` is true; for guests, buyers and sellers it SHALL NOT be in the DOM. Clicking it SHALL open a dialog "Ẩn đánh giá" with a reason textarea limited to 500 characters; the submit button "Ẩn đánh giá" SHALL be disabled while the trimmed reason is empty. On submit `hideReviewAction(reviewId, listingId, reason)` SHALL call `EngagementService.HideReview` once, and on success the dialog SHALL close, a toast "Đã ẩn đánh giá." SHALL show and the PDP SHALL be revalidated so the review is gone from the list and the summary. `hideReviewAction` SHALL reject a trimmed reason of 0 or more than 500 characters with "Lý do ẩn phải từ 1 đến 500 ký tự." without a gateway call, and SHALL NOT take any role from the client.

#### Scenario: Admin sees the hide button on every review
- **WHEN** an admin opens a PDP listing 3 reviews
- **THEN** 3 elements with `data-testid="hide-review"` are present, one per review
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewList.test.tsx` `shows hide only to admins`

#### Scenario: Non-admins never see the hide button
- **WHEN** a guest, a buyer and a seller each open the same PDP
- **THEN** the page HTML contains zero `data-testid="hide-review"` elements and no "Ẩn đánh giá" dialog
- **VERIFIED BY**: `ReviewList.test.tsx` `shows hide only to admins`; `team-frontend/src/app/(shop)/listing/[id]/page.test.tsx` `passes isAdmin only for the admin scope`

#### Scenario: Hide succeeds and the review disappears
- **WHEN** an admin clicks "Ẩn" on a review, types "Spam quảng cáo" and confirms
- **THEN** the dialog closes, the toast "Đã ẩn đánh giá." shows, and the review is absent from the list and from the count on the first re-render after the action resolves, without a manual reload
- **AND** when a guest then opens the same PDP (a step of the same e2e scenario) the review is absent for the guest too
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `Admin hides a review and it disappears for everyone`

#### Scenario: Hide action sends one call and revalidates
- **WHEN** `hideReviewAction("r1", "l1", "Spam quảng cáo")` runs and the gateway answers OK
- **THEN** exactly one HideReview call is made with review id `r1` and reason "Spam quảng cáo", `/listing/l1` is revalidated, and the result is `ok`
- **VERIFIED BY**: `team-frontend/src/features/review/actions.test.ts` `hides and revalidates the listing`

#### Scenario: Empty reason is blocked
- **WHEN** the reason is empty or only spaces
- **THEN** the dialog's submit button is disabled, and `hideReviewAction("r1", "l1", "   ")` returns "Lý do ẩn phải từ 1 đến 500 ký tự." with zero gateway calls
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewHideButton.test.tsx` `disables submit on empty reason`; `actions.test.ts` `rejects an empty reason locally`

#### Scenario: Hide reason length is checked in the dialog
- **WHEN** the reason is exactly 500 characters, and then 501 characters
- **THEN** the 500-character reason is sent to the gateway, the textarea refuses character 501 (`maxLength=500`), and `hideReviewAction` with 501 characters returns "Lý do ẩn phải từ 1 đến 500 ký tự." with zero gateway calls
- **VERIFIED BY**: `actions.test.ts` `accepts 500 and rejects 501 characters`; `ReviewHideButton.test.tsx` `limits the textarea to 500`

#### Scenario: A forged call by a non-admin is refused
- **WHEN** the gateway answers `PermissionDenied` to HideReview
- **THEN** the action returns the error "Bạn không có quyền ẩn đánh giá." and does not revalidate
- **AND** the action signature has no role or `isAdmin` parameter
- **VERIFIED BY**: `actions.test.ts` `surfaces PermissionDenied`

#### Scenario: Other hide errors are shown and the review stays
- **WHEN** the gateway answers `NotFound`, `Unavailable` (team-audit down, fail closed) or `Unauthenticated`
- **THEN** the dialog stays open with an inline error: "Đánh giá không còn tồn tại." for `NotFound`, "Chưa ẩn được đánh giá. Vui lòng thử lại sau." for `Unavailable`, "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." for `Unauthenticated`; any other error shows "Ẩn đánh giá thất bại."; the review remains listed
- **VERIFIED BY**: `actions.test.ts` `maps NotFound, Unavailable, Unauthenticated`; `ReviewHideButton.test.tsx` `keeps the dialog open on error`

#### Scenario: Hiding an already hidden review is a quiet success
- **WHEN** an admin hides a review that another admin hid a moment earlier and the gateway answers OK
- **THEN** the dialog closes with the toast "Đã ẩn đánh giá." and the review is absent after revalidation
- **VERIFIED BY**: `actions.test.ts` `treats an OK reply as success`

#### Scenario: Pending state blocks a double submit
- **WHEN** an admin confirms the hide dialog and the call is in flight
- **THEN** the submit button is `isLoading`, the textarea and "Hủy" are disabled, and only one HideReview call is made after two rapid clicks
- **VERIFIED BY**: `ReviewHideButton.test.tsx` `sends one request while pending`

#### Scenario: A non-admin does not get the control in the browser
- **WHEN** a signed-in buyer and a guest open a PDP that has reviews
- **THEN** no "Ẩn" button is visible to either
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/review_integrity_ui.feature` `Buyers and guests do not see the hide control`

### Requirement: Existing UI e2e features seed reviews through completed orders

UI e2e features under `platform-e2e/tests/e2e/features/frontend/` SHALL create reviews only through the shared completed-order fixture (a verified review tied to a completed order), and SHALL NOT call CreateReview without an `order_id`. The scenario "Submitting a review shows pending then success" SHALL submit from `/account/orders` instead of from the PDP. The fixture file is owned by engagement-review-integrity and SHALL NOT be edited by this slice.

#### Scenario: Seeded reviews are verified
- **WHEN** the scenarios of `product_detail_ui.feature`, `rvp_reviews_pagination_ui.feature` and `uif_product_detail.feature` and `uif_foundation.feature` seed reviews
- **THEN** every seeded review is created with a completed order and the PDP shows "✓ Đã mua hàng" on it; the suites pass with the 23, 105, 1 and 2 review seeds unchanged in count
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature` `Reviews paginate at 10`; `platform-e2e/tests/e2e/features/frontend/rvp_reviews_pagination_ui.feature` `A listing with 105 reviews shows 11 pages`; `platform-e2e/tests/e2e/features/frontend/uif_product_detail.feature` `The reviews section renders with its reviews and no AI summary, error text or leftover skeleton`

#### Scenario: Submit-review scenario runs from the orders page
- **WHEN** a buyer with a completed order opens `/account/orders` with the form submission slowed, clicks "Đánh giá", picks 5 stars, types a comment and submits
- **THEN** the submit button is pending (`aria-busy="true"`) and the modal controls are disabled, then a success toast shows, the modal closes and the PDP lists the review
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature` `Submitting a review shows pending then success`

#### Scenario: A foreign or ineligible order is shown as a clear error
- **WHEN** the gateway answers `FailedPrecondition` to a review submitted from the modal (the UI only offers the buyer's own completed orders; foreign orders are covered at API level by engagement-review-integrity)
- **THEN** the modal shows "Chỉ có thể đánh giá sản phẩm trong đơn hàng đã hoàn thành của chính bạn." and stays open
- **VERIFIED BY**: `team-frontend/src/features/review/ReviewModal.test.tsx` `shows the ineligible-order message`

#### Scenario: Seeded reviews belong to the buyer who owns the order
- **WHEN** the seeding Givens create a review through `completed_order_for` and `seed_verified_review`
- **THEN** the review author is the buyer who owns the completed order, never another user, so no seeding step relies on a foreign order being accepted
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/product_detail_ui.feature` `A successful mutation revalidates the page`
