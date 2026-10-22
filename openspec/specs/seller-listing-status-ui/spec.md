# seller-listing-status-ui Specification

## Purpose
The seller center shows each listing's moderation status and reason.

## Requirements

### Requirement: Seller center shows moderation status and reason

The seller listing views SHALL map `PENDING_REVIEW`, `BANNED` and `HELD` to the tags "Chờ duyệt", "Bị khóa" and "Tạm ẩn" next to the existing "Đang bán", "Bản nháp" and "Bị từ chối". The seller edit page SHALL show, from `Listing.moderation`, the reason code label and the admin note for a `REJECTED` or `BANNED` listing, SHALL show a fixed explanation for `HELD` and "Tin đăng đang chờ kiểm duyệt." for `PENDING_REVIEW`, and SHALL render the form read-only (disabled fieldset, no save button) for `BANNED` and `HELD`. A reason code not in the label table SHALL be shown as its raw text. Screen flags SHALL NOT be rendered on any seller surface. The write path SHALL send only `DRAFT` or `PUBLISHED` as the requested status. Every content edit of a `PUBLISHED` listing is re-screened by the service, so any save may end `PENDING_REVIEW` and SHALL be reported as waiting for review. A `FailedPrecondition` on update or delete (a `BANNED` or `HELD` listing) SHALL show a fixed Vietnamese message and never `err.message`.

#### Scenario: Moderation UI: New status tags
- **WHEN** the seller list renders listings with statuses pending_review, banned and held
- **THEN** the tags read "Chờ duyệt", "Bị khóa" and "Tạm ẩn"
- **VERIFIED BY**: `team-frontend/src/features/seller/status.test.tsx` `maps the new listing statuses`

#### Scenario: Moderation UI: Unknown status stays neutral
- **WHEN** a listing has a status label the UI does not know
- **THEN** the tag reads "Chưa xác định" and nothing throws
- **VERIFIED BY**: `team-frontend/src/features/seller/status.test.tsx` `falls back for an unknown status`

#### Scenario: Moderation UI: Rejected listing shows reason and note
- **WHEN** a seller opens the edit page of a `REJECTED` listing with reason `counterfeit` and note "Ảnh là hàng nhái"
- **THEN** the page shows "Tin đăng bị từ chối: Hàng giả. Ảnh là hàng nhái" and the form stays editable
- **VERIFIED BY**: `team-frontend/src/features/listing/ModerationBanner.test.tsx` `shows the rejected reason and note`

#### Scenario: Moderation UI: Banned listing is read-only
- **WHEN** a seller opens the edit page of a `BANNED` listing
- **THEN** the banner shows the reason and note, the form fieldset is disabled and the save button is absent
- **VERIFIED BY**: `team-frontend/src/features/listing/ModerationBanner.test.tsx` `banned renders the form read-only`

#### Scenario: Moderation UI: Held listing is explained
- **WHEN** a seller opens the edit page of a `HELD` listing
- **THEN** the page shows "Tin đăng đang tạm ẩn vì tài khoản của bạn bị hạn chế. Tin sẽ hiển thị lại khi tài khoản được khôi phục." and the form is read-only
- **VERIFIED BY**: `team-frontend/src/features/listing/ModerationBanner.test.tsx` `held renders the explanation`

#### Scenario: Moderation UI: Pending listing is explained
- **WHEN** a seller opens the edit page of a `PENDING_REVIEW` listing
- **THEN** the page shows "Tin đăng đang chờ kiểm duyệt."
- **VERIFIED BY**: `team-frontend/src/features/listing/ModerationBanner.test.tsx` `pending shows the waiting banner`

#### Scenario: Moderation UI: Unknown reason code is shown raw
- **WHEN** the moderation reason code is `new_reason_x`
- **THEN** the banner shows "new_reason_x" and does not throw
- **VERIFIED BY**: `team-frontend/src/features/listing/ModerationBanner.test.tsx` `falls back to the raw reason code`

#### Scenario: Moderation UI: Screen flags never reach a seller surface
- **WHEN** a listing carries `moderation.screen_flags = ["keyword:thuoc la"]`
- **THEN** no seller view model contains the flags and the seller edit page text does not contain "keyword:"
- **VERIFIED BY**: `team-frontend/src/lib/gateway/listings.test.ts` `seller mapping drops screen flags`

#### Scenario: Moderation UI: A save that ends pending is explained
- **WHEN** a seller saves a new listing or an edit of a `PUBLISHED` listing requested as `PUBLISHED` and the response listing status is `PENDING_REVIEW`
- **THEN** the form shows "Tin đăng đã được gửi và đang chờ kiểm duyệt." instead of "đang bán"
- **VERIFIED BY**: `team-frontend/src/features/listing/actions.test.ts` `a pending result is reported as waiting for review`

#### Scenario: Moderation UI: Write to a banned or held listing is refused
- **WHEN** the gateway answers `FailedPrecondition` to a seller update
- **THEN** the form shows "Tin đăng đang bị khóa hoặc tạm ẩn nên không thể chỉnh sửa." and never `err.message`
- **VERIFIED BY**: `team-frontend/src/features/listing/actions.test.ts` `maps failed precondition on update`

#### Scenario: Moderation UI: Delete of a banned or held listing is refused
- **WHEN** the gateway answers `FailedPrecondition` to a seller delete
- **THEN** the action returns "Tin đăng đang bị khóa hoặc tạm ẩn nên không thể xoá." and never `err.message`, and an unknown error returns "Xoá sản phẩm thất bại."
- **VERIFIED BY**: `team-frontend/src/features/listing/actions.test.ts` `maps failed precondition on delete`

#### Scenario: Moderation UI: Seller sees the pending status after publishing a flagged listing
- **WHEN** a seller publishes a listing whose title contains the mock marker term
- **THEN** the seller list shows the tag "Chờ duyệt" and the edit page shows "Tin đăng đang chờ kiểm duyệt."
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_listing_status_ui.feature` `Moderation UI: Seller sees the pending status after publishing a flagged listing`

#### Scenario: Moderation UI: Seller sees a clean listing as selling
- **WHEN** a seller publishes a clean listing
- **THEN** the seller list shows the tag "Đang bán" and the listing opens for an anonymous visitor
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_listing_status_ui.feature` `Moderation UI: Seller sees a clean listing as selling`

#### Scenario: Moderation UI: Seller edits a rejected listing and it returns to review
- **WHEN** an admin has rejected a listing and its seller saves an edit
- **THEN** the seller list shows "Chờ duyệt" and the listing is still NOT_FOUND for an anonymous visitor
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_listing_status_ui.feature` `Moderation UI: Seller edits a rejected listing and it returns to review`

#### Scenario: Moderation UI: The seller never sees the screen flags
- **WHEN** a seller opens the edit page of a flagged pending listing in the browser
- **THEN** the page text does not contain "keyword:" or "ai:"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_listing_status_ui.feature` `Moderation UI: The seller never sees the screen flags`
