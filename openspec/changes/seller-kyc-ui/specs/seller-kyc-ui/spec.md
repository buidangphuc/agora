## ADDED Requirements

### Requirement: Seller sees their verification status

The seller app SHALL provide `/seller/verification` showing the signed-in seller's own status from `GetVerificationStatus`, in Vietnamese,
with a text label for each of `NOT_SUBMITTED`, `PENDING`, `VERIFIED` and `REJECTED`, and the reject reason for `REJECTED`.

#### Scenario: A seller who never submitted sees NOT_SUBMITTED and the form
- **WHEN** a seller with no submission opens `/seller/verification`
- **THEN** the page shows the label "Chưa gửi hồ sơ" and the upload form, and 0 requests use a user id other than the caller's
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A pending submission hides the form
- **WHEN** a seller whose latest submission is `PENDING` opens the page
- **THEN** the page shows "Đang chờ duyệt" and renders 0 file inputs
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A verified seller sees the verified state
- **WHEN** a seller whose latest submission is `VERIFIED` opens the page
- **THEN** the page shows "Đã xác minh" and renders 0 file inputs
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A rejected submission shows the reason and allows resubmission
- **WHEN** a seller whose latest submission is `REJECTED` with `reject_reason_code` `document_unreadable` opens the page
- **THEN** the page shows "Bị từ chối" with the Vietnamese text for that code and renders the upload form again
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: An unknown reject reason code falls back to a generic text
- **WHEN** the status response carries a `reject_reason_code` the UI has no text for
- **THEN** the page shows the generic text "Hồ sơ chưa đạt yêu cầu, vui lòng gửi lại" and never renders the raw code
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Status read failure is shown as an error, not as not-submitted
- **WHEN** `GetVerificationStatus` fails with `UNAVAILABLE`
- **THEN** the page shows an error Alert with a "Thử lại" link and renders 0 upload forms
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A guest is sent to login
- **WHEN** an anonymous visitor opens `/seller/verification`
- **THEN** the response redirects to the login page with the return path `/seller/verification`
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A buyer without seller scope cannot use the page
- **WHEN** a signed-in user without the `listing.write` scope opens `/seller/verification`
- **THEN** the seller access-denied view renders and 0 verification calls are made
- **VERIFIED BY**: TBD by e2e-author

### Requirement: Seller uploads ID documents through presigned URLs

The upload form SHALL accept 1 to 3 files of type image/jpeg, image/png or application/pdf, each 1 byte to 5 MiB, SHALL require the
explicit consent checkbox, and SHALL upload each file by `CreateKycUploadUrl` followed by a browser `PUT` to the returned URL, then call
`SubmitKyc` with the first `doc_ref` and the rest as `additional_doc_refs`.

#### Scenario: Three valid files are uploaded and submitted
- **WHEN** a seller selects 3 valid files, ticks the consent checkbox and presses "Gửi hồ sơ xác minh"
- **THEN** 3 `CreateKycUploadUrl` calls, 3 `PUT` requests with matching `Content-Type` and `Content-Length`, and exactly 1 `SubmitKyc` call with 1 `doc_ref` and 2 `additional_doc_refs` occur, and the page then shows "Đang chờ duyệt"
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Consent unticked blocks submission
- **WHEN** a seller selects a valid file but leaves the consent checkbox unticked
- **THEN** the submit button is disabled and 0 `CreateKycUploadUrl` calls are made
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A file over 5 MiB is rejected before upload
- **WHEN** a seller selects a file of 5 MiB + 1 byte
- **THEN** the form shows "Tệp tối đa 5 MiB" for that file, the submit button is disabled and 0 network calls are made
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A file of exactly 5 MiB is accepted
- **WHEN** a seller selects a PNG of exactly 5 MiB (5242880 bytes)
- **THEN** the form shows no size error and submit is enabled once consent is ticked
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A wrong file type or an empty file is rejected
- **WHEN** a seller selects a `.gif` file or a 0-byte file
- **THEN** the form shows "Chỉ nhận JPG, PNG hoặc PDF" or "Tệp rỗng" respectively and submit is disabled
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: More than three files are refused
- **WHEN** a seller selects 4 files
- **THEN** the form keeps only the limit message "Tối đa 3 tệp", submit is disabled and 0 network calls are made
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A failed PUT stops the submission
- **WHEN** the browser `PUT` of the second file fails or the presigned URL has expired
- **THEN** `SubmitKyc` is not called, the form shows "Tải tệp thất bại, vui lòng thử lại" and the seller can retry without re-selecting files
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: The server refuses a doc_ref
- **WHEN** `SubmitKyc` fails with `INVALID_ARGUMENT` or `PERMISSION_DENIED` because a doc_ref was not issued to the caller
- **THEN** the form shows "Hồ sơ không hợp lệ, vui lòng tải lại tệp" and the status stays unchanged
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Submit cannot be sent twice
- **WHEN** a seller double-clicks the submit button
- **THEN** exactly 1 `SubmitKyc` call is made
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Image bytes and keys never reach logs or the server action payload
- **WHEN** an upload completes
- **THEN** the Server Action requests contain only content type, size, document type and `doc_ref` strings (0 file bytes), and the page DOM contains no presigned URL or `doc_ref`
- **VERIFIED BY**: TBD by e2e-author

### Requirement: Seller navigation and payout messaging point to verification

`SELLER_NAV` SHALL include "Xác minh danh tính" linking to `/seller/verification`, and the wallet page SHALL explain a blocked payout and link to
the verification page.

#### Scenario: Nav lists the verification page
- **WHEN** a seller opens any `/seller` page
- **THEN** the navigation contains exactly 1 entry "Xác minh danh tính" with href `/seller/verification`
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Wallet shows a notice while not verified
- **WHEN** a seller whose status is not `VERIFIED` opens `/seller/wallet`
- **THEN** the page shows the notice "Cần xác minh danh tính để rút tiền" with a link to `/seller/verification`
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A refused payout shows the verification message
- **WHEN** a seller confirms a payout and the gateway returns `FAILED_PRECONDITION` "seller verification required"
- **THEN** the modal shows "Cần xác minh danh tính để rút tiền" with a link to `/seller/verification`, the balance is unchanged and the raw server text is not shown
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Verification outage during payout shows a retry message
- **WHEN** the payout call fails with `UNAVAILABLE`
- **THEN** the modal shows "Chưa kiểm tra được trạng thái xác minh, vui lòng thử lại sau" and the confirm button is enabled again
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: A verified seller sees no notice and can pay out
- **WHEN** a `VERIFIED` seller with a balance above 0 opens `/seller/wallet` and confirms a payout
- **THEN** no verification notice is rendered and the payout succeeds with the toast "Đã tạo lệnh rút tiền"
- **VERIFIED BY**: TBD by e2e-author

### Requirement: KYC exists only in the seller app

team-frontend SHALL NOT serve `/account/verification` and SHALL NOT link to it.

#### Scenario: The buyer route is gone
- **WHEN** a signed-in buyer requests `/account/verification` on the buyer app
- **THEN** the response is 404
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Buyer navigation has no verification link
- **WHEN** a signed-in buyer opens any `/account/*` page
- **THEN** the account nav and the top bar contain 0 links to `/account/verification`
- **VERIFIED BY**: TBD by e2e-author

### Requirement: Shop page shows a public verified badge

`/shop/[id]` SHALL show a "Đã xác minh" badge if and only if `GetPublicBadges` returns `verified=true` for that seller, and SHALL expose no
other verification state.

#### Scenario: Verified seller's shop shows the badge
- **WHEN** anyone, including an anonymous visitor, opens the shop page of a `VERIFIED` seller
- **THEN** the page contains exactly 1 "Đã xác minh" badge
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Non-verified shops show no badge and no status
- **WHEN** anyone opens the shop page of a seller who is `PENDING`, `REJECTED` or `NOT_SUBMITTED`
- **THEN** the page contains 0 badges and none of the words "Đang chờ duyệt", "Bị từ chối" or "Chưa gửi hồ sơ"
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: Badge lookup failure does not break the page
- **WHEN** `GetPublicBadges` fails or times out
- **THEN** the shop page still renders with HTTP 200 and 0 badges
- **VERIFIED BY**: TBD by e2e-author

#### Scenario: The badge call asks for the shop seller only
- **WHEN** the shop page is rendered
- **THEN** exactly 1 `GetPublicBadges` call is made with 1 user id equal to the shop's seller id
- **VERIFIED BY**: TBD by e2e-author
