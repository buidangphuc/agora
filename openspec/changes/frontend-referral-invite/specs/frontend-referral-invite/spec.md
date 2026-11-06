## ADDED Requirements

### Requirement: The referral page creates and copies an invite link

The `/account/referral` page SHALL offer "Chia sẻ link mời", which calls `SharingService.CreateShareLink` with
`target_type="referral"`, `target_id=<the buyer's own code>` and `utm={utm_source: "referral", utm_medium: "invite"}`
and copies `<origin>/s/<shortCode>` to the clipboard.

#### Scenario: S1 Buyer copies the invite link
- **WHEN** a signed-in buyer with a referral code taps "Chia sẻ link mời"
- **THEN** the system SHALL call `CreateShareLink("referral", <code>, {utm_source: "referral", utm_medium: "invite"})` once
- **AND** the system SHALL show the toast "Đã sao chép link mời."
- **AND** the clipboard SHALL hold `<origin>/s/<shortCode>` with the short code returned by the call

#### Scenario: S2 Buyer has no code yet (boundary)
- **WHEN** a signed-in buyer without a referral code opens the page
- **THEN** the system SHALL render "Chia sẻ link mời" disabled and keep the "generate code" button
- **AND** the system SHALL call no `CreateShareLink`

#### Scenario: S3 Share link creation fails (error)
- **WHEN** `CreateShareLink` fails with an error
- **THEN** the system SHALL show the error toast "Không tạo được link mời. Vui lòng thử lại." and leave the clipboard unchanged

#### Scenario: S4 Guest is sent to login (permission)
- **WHEN** a guest opens `/account/referral`
- **THEN** the system SHALL redirect to the login page with `returnUrl=/account/referral` and render no code, link or totals

### Requirement: The short-link resolver keeps utm and sets the referral cookie

`GET /s/<code>` SHALL resolve the link with `SharingService.ResolveShareLink` and answer a 307 redirect whose query carries every
stored `utm_*` key. A `referral` link SHALL redirect to `/register?ref=<code>` and set the cookie `agora_ref`; no other target type sets it.

#### Scenario: S5 Referral link redirects to register with attribution
- **WHEN** a visitor opens `/s/<shortCode>` for a `referral` link with code `ABC123` and utm `{utm_source: "referral", utm_medium: "invite"}`
- **THEN** the system SHALL answer 307 with Location `/register?ref=ABC123&utm_medium=invite&utm_source=referral` (utm keys sorted)
- **AND** the response SHALL set `agora_ref=ABC123` with `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=604800` and `Secure` per `isSecureRequest`

#### Scenario: S6 Listing and shop links keep utm and set no cookie
- **WHEN** a visitor opens `/s/<shortCode>` for a `listing` link with utm `{utm_source: "share"}`
- **THEN** the system SHALL answer 307 with Location `/listing/<id>?utm_source=share` and no `agora_ref` Set-Cookie
- **AND** a `shop` or `seller` link SHALL redirect to `/shop/<id>` with its utm kept and no `agora_ref`

#### Scenario: S7 Unknown short code (error)
- **WHEN** a visitor opens `/s/<code>` and `ResolveShareLink` fails or returns no link
- **THEN** the system SHALL answer 307 with Location `/` and set no cookie

#### Scenario: S8 Unknown target type (error)
- **WHEN** the resolved link has a target type other than `listing`, `shop`, `seller` or `referral`
- **THEN** the system SHALL answer 307 with Location `/`, no utm and no cookie

#### Scenario: S9a Non-utm key dropped (boundary)
- **WHEN** the stored utm contains a key not starting with `utm_`
- **THEN** the system SHALL answer 307 with a Location that omits that key and keeps the `utm_*` keys

#### Scenario: S9b Unsafe referral id (error, permission)
- **WHEN** a `referral` link's target id does not match `^[A-Za-z0-9_-]{1,32}$`
- **THEN** the system SHALL answer 307 with Location `/` and set no cookie

### Requirement: Register sends the referral code from the link or the cookie

`/register` SHALL pass `?ref` to the form, and `registerAction` SHALL send `referral_code` in `AuthService.Register` using, in order,
the `ref` form field then the `agora_ref` cookie, each only when it matches `^[A-Za-z0-9_-]{1,32}$`. A successful register SHALL delete `agora_ref`.
Registration SHALL never fail because of the referral code, and `returnUrl` SHALL still be honoured.

#### Scenario: S10 Register with ?ref
- **WHEN** a visitor on `/register?ref=ABC123` submits a valid username and password
- **THEN** the system SHALL call `Register` with `referral_code="ABC123"` and redirect to the safe `returnUrl` or `/`

#### Scenario: S11 Register with only the cookie, then cookie cleared
- **WHEN** a visitor with cookie `agora_ref=ABC123` and no `ref` field registers successfully
- **THEN** the system SHALL send `referral_code="ABC123"`
- **AND** the response SHALL delete `agora_ref` (Set-Cookie with `Max-Age=0`)

#### Scenario: S12 Field wins over cookie (boundary)
- **WHEN** the form field `ref=NEW1` and the cookie `agora_ref=OLD1` are both present
- **THEN** the system SHALL send `referral_code="NEW1"`

#### Scenario: S13 Malformed code is ignored (error)
- **WHEN** the `ref` field is `bad code!` or longer than 32 characters and no valid cookie exists
- **THEN** the system SHALL send an empty `referral_code` and still register the user

#### Scenario: S14 Failed register keeps the cookie (error)
- **WHEN** `Register` fails with AlreadyExists
- **THEN** the system SHALL return "Tên đăng nhập đã tồn tại." and SHALL NOT delete `agora_ref`

#### Scenario: S15 No code, no field
- **WHEN** a visitor with neither `ref` nor cookie registers
- **THEN** the system SHALL send an empty `referral_code` and issue no Set-Cookie for `agora_ref`

#### Scenario: S16 returnUrl still honoured
- **WHEN** `/register?returnUrl=/cart&ref=ABC123` is submitted successfully
- **THEN** the system SHALL redirect to `/cart` and send `referral_code="ABC123"`

### Requirement: Rewards are shown in xu with their status

`/account/referral` SHALL format reward amounts with `formatXu` (integer, vi-VN grouping, suffix " xu") and never with `formatPrice`.
It SHALL show `pending_xu` as "Xu đang chờ", `released_xu` as "Xu đã nhận", and list each reward with its status.

#### Scenario: S17 Totals in xu
- **WHEN** `GetMyReferral` returns `pending_xu=10000` and `released_xu=20000`
- **THEN** the system SHALL render "Xu đang chờ" as `10.000 xu` and "Xu đã nhận" as `20.000 xu`, and the page SHALL contain no "đ" currency amount

#### Scenario: S18 Reward timeline statuses
- **WHEN** `ListReferralRewards` returns one PENDING reward (release_at set), one RELEASED and one VOID
- **THEN** the system SHALL show badges "Đang chờ" with "Dự kiến nhận: <release_at>" formatted dd/MM/yyyy in vi-VN and Asia/Ho_Chi_Minh, "Đã nhận" and "Đã hủy" respectively
- **AND** the VOID reward amount SHALL be rendered struck through and not added to any total

#### Scenario: S19 No rewards (boundary)
- **WHEN** `GetMyReferral` returns zero for both totals and the list is empty
- **THEN** the system SHALL render `0 xu` twice and the text "Chưa có phần thưởng nào."

#### Scenario: S20 Rewards read fails (error)
- **WHEN** `ListReferralRewards` fails
- **THEN** the system SHALL render the code and totals and an empty timeline with "Chưa có phần thưởng nào.", not an error page

#### Scenario: S21 Totals are the caller's own (permission)
- **WHEN** buyer A is signed in
- **THEN** the system SHALL show only the totals and rewards returned for A's own session and call no RPC with another user id

### Requirement: The manual redeem form explains the 7-day rule

The redeem form on `/account/referral` SHALL state the rule and SHALL surface a `FAILED_PRECONDITION` refusal in Vietnamese.

#### Scenario: S22 Rule text shown
- **WHEN** a signed-in buyer opens the page
- **THEN** the system SHALL render under the redeem field "Chỉ nhập được trong 7 ngày đầu và khi bạn chưa có đơn hoàn thành."

#### Scenario: S23 Redeem refused (error)
- **WHEN** `RedeemReferral` fails with `FAILED_PRECONDITION` (account older than 7 days, completed order, second redeem or own code)
- **THEN** the system SHALL show the server's Vietnamese message in the error toast and under the field
- **AND** when that message is empty the system SHALL show "Không thể nhập mã: tài khoản đã quá 7 ngày, đã có đơn hoàn thành, đã nhập mã hoặc mã là của chính bạn."

#### Scenario: S24 Redeem accepted
- **WHEN** `RedeemReferral` succeeds
- **THEN** the system SHALL show the toast "Đã ghi nhận mã giới thiệu.", clear the field and fire `trackEcommerce("referral_redeem")` once

#### Scenario: S25 Guest cannot redeem (permission)
- **WHEN** a guest session submits `redeemReferralAction`
- **THEN** the system SHALL show the gateway refusal as an error toast and keep the field value
