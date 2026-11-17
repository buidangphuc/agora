# frontend-account-privacy Specification

## Purpose
TBD - created by archiving change frontend-account-privacy. Update Purpose after archive.

## Requirements

### Requirement: The privacy page is a signed-in account page

team-frontend SHALL serve `/account/privacy` inside `AccountShell` with menu key `privacy` labelled "Quyền riêng tư",
reachable from the account menu and the `ConsumerShell` account links. A guest SHALL be redirected to `/login` with the
page as return URL. A failed read SHALL show an inline error with a retry for that card only.

#### Scenario: PV1 Signed-in user reaches the page from the account menu
- **WHEN** a signed-in buyer clicks "Quyền riêng tư" in the account menu
- **THEN** the browser lands on `/account/privacy`, the heading "Quyền riêng tư" shows and the menu entry has `aria-current="page"`
- **VERIFIED BY**: `team-frontend/src/features/account/AccountShell.test.tsx` `lists the privacy entry and marks it current`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV1 Signed-in user reaches the page from the account menu`

#### Scenario: PV2 Guest is sent to login
- **WHEN** a guest opens `/account/privacy`
- **THEN** the response redirects to `/login` with the return URL `/account/privacy` and 0 gateway calls are made
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `redirects a guest to login`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `Guest is sent to login`

#### Scenario: PV3 One failing read does not blank the page
- **WHEN** `ListMyConsents` fails with `Unavailable` and `GetAccountDeletionStatus` succeeds
- **THEN** the consent card shows "Không thể tải lịch sử đồng ý." with a retry button and the deletion and export cards still render
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `a failed consent read keeps the other cards`

#### Scenario: PV29 A guest cannot reach any privacy data
- **WHEN** a guest opens `/account/privacy` or calls a privacy action without a session
- **THEN** the page redirects to `/login` or the action is refused `Unauthenticated`, and no consent, export or deletion data is returned
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `redirects a guest to login`

### Requirement: The user sees and changes their consent with a server record

The consent card SHALL list the records from `ListMyConsents` newest first, 20 per page, with purpose, granted or
withdrawn, version and time, and SHALL show the current value per purpose. Saving SHALL call `RecordConsent` with one
record per purpose whose value changed (source `account`, version `1`) and only after the call succeeds SHALL write the
`agora_consent` cookie. THE SYSTEM SHALL make no call when nothing changed. The page SHALL NOT offer to edit TERMS,
PRIVACY_POLICY or SELLER_TERMS records.

#### Scenario: PV4 History is listed newest first and paged
- **WHEN** a user with 25 consent records opens `/account/privacy`
- **THEN** the history shows 20 rows ordered newest first, the first row is the newest record and a pagination link to page 2 shows 5 more rows
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `lists consents newest first, 20 per page`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV4 History is listed newest first and paged`

#### Scenario: PV5 Saving a change records one row per changed purpose
- **WHEN** a user with analytics granted switches analytics off and marketing on and saves
- **THEN** `RecordConsent` is called once with exactly 2 records (analytics granted false, marketing granted true, source `account`), then the cookie holds `a=0&m=1`, a toast "Đã lưu lựa chọn của bạn." shows and the history gains 2 rows
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `records only the changed purposes with source account`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV5 Saving a change records one row per changed purpose`

#### Scenario: PV6 Saving without a change makes no call
- **WHEN** a user opens the page and clicks save without toggling anything
- **THEN** 0 `RecordConsent` calls are made and the save button is disabled until a toggle changes
- **VERIFIED BY**: `team-frontend/src/features/privacy/ConsentForm.test.tsx` `save is disabled until a purpose changes`

#### Scenario: PV7 A failed record leaves the cookie unchanged
- **WHEN** `RecordConsent` fails `Unavailable` after the user toggles personalization
- **THEN** the cookie value is unchanged, an inline Alert "Chưa lưu được lựa chọn. Vui lòng thử lại." shows and the toggle keeps the user's choice
- **VERIFIED BY**: `team-frontend/src/features/privacy/ConsentForm.test.tsx` `does not write the cookie when the record fails`

#### Scenario: PV8 Legal acceptance rows are read-only
- **WHEN** the history contains TERMS, PRIVACY_POLICY and SELLER_TERMS records
- **THEN** each shows its Vietnamese label with the accepted version and no toggle exists for it
- **VERIFIED BY**: `team-frontend/src/features/privacy/ConsentForm.test.tsx` `renders toggles only for the three banner purposes`

#### Scenario: PV9 An empty history shows an empty state
- **WHEN** a legacy user with no consent records opens the page
- **THEN** the history shows "Chưa có lịch sử đồng ý." and the toggles show the cookie values
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `empty history shows the empty state`

#### Scenario: PV10 An expired session on save asks to sign in again
- **WHEN** `RecordConsent` fails `Unauthenticated`
- **THEN** the action returns "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." and the cookie is unchanged
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps Unauthenticated on consent save`

### Requirement: The user downloads their data

The export card SHALL call `ExportMyData` from a server action on click and deliver the returned bytes as a browser
file named `agora-du-lieu-<YYYY-MM-DD>.json` with content type `application/json`. The app SHALL NOT store, cache or
log the content. A failure SHALL show a fixed Vietnamese message and leave the button usable. The card SHALL link the
in-app views of orders, reviews and chats.

#### Scenario: PV11 Export downloads one JSON file
- **WHEN** a signed-in user clicks "Tải dữ liệu của tôi"
- **THEN** the browser receives one file named `agora-du-lieu-<date>.json` whose JSON has `account.user_id` equal to the user's id
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `export returns filename and content from the response`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV11 Export downloads one JSON file`

#### Scenario: PV12 Export failure is shown and retryable
- **WHEN** `ExportMyData` fails `Unavailable`
- **THEN** an Alert "Chưa tải được dữ liệu. Vui lòng thử lại sau." shows, no file is produced and the button is enabled again
- **VERIFIED BY**: `team-frontend/src/features/privacy/ExportButton.test.tsx` `shows an error and re-enables the button`

#### Scenario: PV13 Export content is never retained or logged
- **WHEN** the export action succeeds
- **THEN** the action writes 0 cookies, calls no `revalidatePath` and logs 0 lines containing the content, and the page HTML contains none of the exported values
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `export leaves no trace outside the response`

#### Scenario: PV14 Double click makes one call
- **WHEN** the user clicks the export button twice within the pending state
- **THEN** `ExportMyData` is called once
- **VERIFIED BY**: `team-frontend/src/features/privacy/ExportButton.test.tsx` `blocks a second click while pending`

#### Scenario: PV15 The export card links the in-app views
- **WHEN** the page renders
- **THEN** the export card contains links to `/account/orders`, the user's reviews entry point and `/chat`
- **VERIFIED BY**: `team-frontend/src/app/(shop)/account/privacy/page.test.tsx` `export card links orders and chats`

#### Scenario: PV30 An unauthenticated export is refused
- **WHEN** `ExportMyData` answers `Unauthenticated` or `PermissionDenied`
- **THEN** the action returns "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." and no file is produced
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps Unauthenticated on every action`

### Requirement: The user requests and cancels account deletion

The deletion card SHALL read `GetAccountDeletionStatus`. With nothing pending it SHALL offer "Xóa tài khoản", which
opens a modal asking for the password and calling `RequestAccountDeletion` with that password only. On success it SHALL
show the pending panel with `erase_after` formatted `vi-VN` and a "Hủy yêu cầu xóa" button calling
`CancelAccountDeletion`. On `FAILED_PRECONDITION` it SHALL show the blocker message and keep the account unchanged. The
password SHALL NOT be stored, logged or returned to the page.

#### Scenario: PV16 Confirming with the password schedules deletion
- **WHEN** a buyer with no open obligations enters the correct password in the deletion modal and confirms
- **THEN** the page shows the pending panel with an erase time 48 h (within 5 s) after the request and the "Hủy yêu cầu xóa" button
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `request deletion sends only the password`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV16 Confirming with the password schedules deletion`

#### Scenario: PV17 Wrong password schedules nothing
- **WHEN** the gateway answers `PermissionDenied` to `RequestAccountDeletion`
- **THEN** the modal stays open with "Mật khẩu không đúng.", the password field is cleared and the status read still shows nothing pending
- **VERIFIED BY**: `team-frontend/src/features/privacy/DeleteAccountModal.test.tsx` `wrong password keeps the modal open and clears the field`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV17 Wrong password schedules nothing`

#### Scenario: PV18 Empty password is blocked locally
- **WHEN** the user clicks confirm with an empty password
- **THEN** the confirm button is disabled and 0 gateway calls are made
- **VERIFIED BY**: `team-frontend/src/features/privacy/DeleteAccountModal.test.tsx` `confirm is disabled for an empty password`

#### Scenario: PV19 An open obligation shows the reason
- **WHEN** the gateway answers `FailedPrecondition` with a message naming open orders
- **THEN** the modal shows "Chưa thể xóa tài khoản: " followed by that message, no pending panel appears and the account stays usable
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps FailedPrecondition to prefix plus category`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV19 An open obligation shows the reason`

#### Scenario: PV20 Obligation service outage fails closed
- **WHEN** the gateway answers `Unavailable` to `RequestAccountDeletion`
- **THEN** the modal shows "Chưa kiểm tra được đơn hàng và số dư của bạn. Vui lòng thử lại sau." and no pending panel appears
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps Unavailable on request deletion`

#### Scenario: PV21 Pending deletion can be cancelled
- **WHEN** a user whose deletion is pending clicks "Hủy yêu cầu xóa" and confirms
- **THEN** `CancelAccountDeletion` is called once, the panel returns to the "Xóa tài khoản" offer and the toast "Đã hủy yêu cầu xóa tài khoản." shows
- **VERIFIED BY**: `team-frontend/src/features/privacy/DeletionCard.test.tsx` `cancel returns to the delete offer`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV21 Pending deletion can be cancelled`

#### Scenario: PV22 Cancel with nothing pending is explained
- **WHEN** `CancelAccountDeletion` fails `FailedPrecondition` because the window already ended or was cancelled elsewhere
- **THEN** the action returns "Không có yêu cầu xóa tài khoản đang chờ." and the page re-reads the status
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps FailedPrecondition on cancel`

#### Scenario: PV23 A repeated request keeps the original time
- **WHEN** a user with deletion pending opens the page in a second tab and sees the pending panel
- **THEN** the second tab offers only the cancel button and no "Xóa tài khoản" button
- **VERIFIED BY**: `team-frontend/src/features/privacy/DeletionCard.test.tsx` `pending state hides the delete offer`

#### Scenario: PV24 The password never leaves the action
- **WHEN** `requestDeletionAction` runs with success or failure
- **THEN** its return value, the logs and the revalidated page contain 0 occurrences of the password
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `never returns or logs the password`

#### Scenario: PV25 Double submit makes one request
- **WHEN** the user clicks confirm twice while the first call is pending
- **THEN** `RequestAccountDeletion` is called once
- **VERIFIED BY**: `team-frontend/src/features/privacy/DeleteAccountModal.test.tsx` `blocks a double submit`

#### Scenario: PV31 Deletion by a session without rights is refused
- **WHEN** `RequestAccountDeletion` answers `Unauthenticated`
- **THEN** the modal shows "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." and no pending panel appears
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps Unauthenticated on every action`

### Requirement: Privacy actions require a user session and hide technical detail

Every privacy action SHALL call the gateway with the caller's session token, SHALL send no user id, and SHALL map errors
to the fixed Vietnamese texts of the design; the raw gateway message SHALL be shown only for the deletion blocker.

#### Scenario: PV26 A call without a session is refused
- **WHEN** an action runs and the gateway answers `Unauthenticated`
- **THEN** the action returns "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại." and shows no technical text
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `maps Unauthenticated on every action`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/account_privacy_ui.feature` `PV26 A call without a session is refused`

#### Scenario: PV27 No request carries a user id
- **WHEN** any privacy action calls the gateway
- **THEN** the request message has no user id field set and the Authorization header carries the session token
- **VERIFIED BY**: `team-frontend/src/lib/gateway/privacy.test.ts` `sends the session token and no user id`

#### Scenario: PV28 Unknown failures hide detail
- **WHEN** an action fails with an error that is not a Connect error or has an unmapped code
- **THEN** the action returns its fixed failure text and the text contains no gateway message
- **VERIFIED BY**: `team-frontend/src/features/privacy/actions.test.ts` `unmapped errors show only the fixed text`
