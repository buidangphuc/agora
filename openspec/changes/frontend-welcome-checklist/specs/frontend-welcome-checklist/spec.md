## ADDED Requirements

### Requirement: Vouchers page lists the buyer's own vouchers

For a signed-in buyer the `/vouchers` page SHALL render a "Voucher của bạn" section above the platform list, built from
`VoucherService.ListMyVouchers`, showing for each voucher its code, discount, minimum spend and expiry. The section
SHALL NOT render and `ListMyVouchers` SHALL NOT be called for an anonymous visitor.

#### Scenario: W1 Own vouchers listed above the platform list
- **WHEN** a signed-in buyer holding one active `NEW_USER` voucher opens `/vouchers`
- **THEN** the system SHALL render a "Voucher của bạn" section containing that voucher's code, amount and expiry before the platform voucher list

#### Scenario: W2 No own vouchers (boundary)
- **WHEN** a signed-in buyer whose `ListMyVouchers` returns an empty list opens `/vouchers`
- **THEN** the system SHALL render no "Voucher của bạn" heading and the platform list unchanged

#### Scenario: W3 Read failure keeps the page
- **WHEN** `ListMyVouchers` fails with Unavailable while the buyer opens `/vouchers`
- **THEN** the system SHALL render the platform list with HTTP 200 and no "Voucher của bạn" section

#### Scenario: W4 Anonymous visitor
- **WHEN** an anonymous visitor opens `/vouchers`
- **THEN** the system SHALL render the platform list, no "Voucher của bạn" section, and issue no `ListMyVouchers` call

#### Scenario: W5 Another buyer's voucher is never shown (permission)
- **WHEN** buyer A, who holds a user-bound voucher, and buyer B each open `/vouchers`
- **THEN** the system SHALL render A's voucher code only in A's page and never in B's page

### Requirement: Checkout voucher picker offers the buyer's own vouchers

In the payment step the platform voucher slot of `VoucherModal` SHALL list the buyer's own active vouchers from
`ListMyVouchers` before the platform vouchers, deduplicated by code.

#### Scenario: W6 Own voucher offered and applied
- **WHEN** a buyer holding a `NEW_USER` voucher opens the payment step and applies it from the picker
- **THEN** the system SHALL include the voucher code in the platform slot list and send it as `platformVoucherCode` in the checkout quote

#### Scenario: W22 Anonymous checkout visitor (permission)
- **WHEN** an anonymous visitor reaches the checkout route
- **THEN** the system SHALL redirect to `/login` and issue no `ListMyVouchers` call

#### Scenario: W7 Picker survives a read failure
- **WHEN** `ListMyVouchers` fails while the payment step renders
- **THEN** the system SHALL render the picker with the platform vouchers only and no error banner

#### Scenario: W8 Ineligible voucher is refused by the quote
- **WHEN** the buyer applies their own voucher while the cart total is below its minimum spend
- **THEN** the system SHALL show the quote's invalid-voucher state returned by the order service and not mark the code as applied

### Requirement: Welcome banner for accounts under 14 days old

For a signed-in buyer whose account age is under 14 days, `/` and every `/account/*` page SHALL render a dismissible
banner. The account age SHALL be `now` minus `created_at` from `AuthService.GetMyAccount`. The
banner SHALL show the code, discount amount and expiry of the buyer's `NEW_USER` voucher, or the text
"Voucher đang được chuẩn bị" without a code when `ListMyVouchers` succeeded with no `NEW_USER` voucher and the account is under 1 hour old.
An expired or used `NEW_USER` voucher SHALL show no code and no pending text.

#### Scenario: W9 Banner shows the voucher
- **WHEN** a buyer whose account is 2 days old and who holds a `NEW_USER` voucher opens `/`
- **THEN** the system SHALL render a banner with the voucher code, the amount in đ and the expiry date

#### Scenario: W10 Voucher not yet issued
- **WHEN** a buyer whose account is 1 minute old opens `/account/addresses` and `ListMyVouchers` returns no `NEW_USER` voucher
- **THEN** the system SHALL render the text "Voucher đang được chuẩn bị" and no voucher code

#### Scenario: W11 Voucher read failed
- **WHEN** `ListMyVouchers` fails for a buyer in the 14-day window
- **THEN** the system SHALL render the checklist without the voucher line and without the text "Voucher đang được chuẩn bị"

#### Scenario: W12 Account age boundary
- **WHEN** the account is 13 days 23 hours old
- **THEN** the system SHALL render the panel
- **AND** when the account is 14 days old the system SHALL render no panel

#### Scenario: W13 Age unknown hides the panel
- **WHEN** `GetMyAccount` fails or returns no `created_at`
- **THEN** the system SHALL render no banner and no checklist
- **AND** the host page SHALL render with HTTP 200

#### Scenario: W14 Dismiss the banner
- **WHEN** the buyer clicks the banner close control
- **THEN** the system SHALL set the cookie `agora_welcome_dismissed=1` (httpOnly, sameSite lax, path `/`, maxAge 1209600, `secure` equal to the `secure` flag of the `session` cookie)
- **AND** the banner SHALL be absent on the next render of `/` and `/account/*`

#### Scenario: W15 Anonymous visitor sees nothing (permission)
- **WHEN** an anonymous visitor opens `/`
- **THEN** the system SHALL render no panel and issue none of `GetMyAccount`, `ListMyVouchers`, `ListFollowedSellers`, `ListAddresses`, `ListBuyerOrders`

### Requirement: Three-step activation checklist

Within the same 14-day window the panel SHALL render a checklist with the steps "Theo dõi một shop" (done when
`ListFollowedSellers` returns at least one seller), "Thêm địa chỉ giao hàng" (done when `ListAddresses` returns at least
one address) and "Đặt đơn đầu tiên" (done when `ListBuyerOrders` returns at least one order whose status is not cancelled). Each undone step SHALL link
to `/account/following`, `/account/addresses` and `/` respectively. The checklist SHALL hide when all three steps are
done or when `agora_checklist_dismissed=1` is present.

#### Scenario: W16 Steps ticked from the reads
- **WHEN** a buyer in the window follows one shop, has no address and no order
- **THEN** the system SHALL render the first step done and the second and third steps not done

#### Scenario: W17 All done hides the checklist
- **WHEN** all three reads return at least one item
- **THEN** the system SHALL render no checklist

#### Scenario: W18 One read fails
- **WHEN** `ListAddresses` fails with Unavailable and the other two reads succeed
- **THEN** the system SHALL render the address step as not done and the other two steps from their own results

#### Scenario: W19 Dismiss the checklist
- **WHEN** the buyer clicks "Ẩn danh sách" on the checklist
- **THEN** the system SHALL set `agora_checklist_dismissed=1` with the same attributes as W14
- **AND** the checklist SHALL be absent afterwards while the banner state is unchanged

#### Scenario: W20 Account older than 14 days
- **WHEN** a buyer whose account is 30 days old opens `/`
- **THEN** the system SHALL render neither banner nor checklist regardless of cookie state

#### Scenario: W21 Slow read times out at 2 s
- **WHEN** `ListAddresses` does not answer within 2 s while the buyer opens `/`
- **THEN** the system SHALL render the address step as not done within 2.5 s of the request, with the other panel parts rendered from their own results

#### Scenario: W24 Expired or used voucher
- **WHEN** `ListMyVouchers` returns a `NEW_USER` voucher whose `ends_at` is in the past or whose `used` equals its `quota` for a buyer in the 14-day window
- **THEN** the system SHALL render no voucher code and not render "Voucher đang được chuẩn bị"

#### Scenario: W25 Cancelled order does not tick the first-order step
- **WHEN** `ListBuyerOrders` returns only orders with status cancelled
- **THEN** the system SHALL render the "Đặt đơn đầu tiên" step as not done

#### Scenario: W26 Pending text expires with the account age (boundary)
- **WHEN** a buyer whose account is 2 hours old has no `NEW_USER` voucher in a successful `ListMyVouchers`
- **THEN** the system SHALL render no voucher line and not render "Voucher đang được chuẩn bị"

#### Scenario: W27 Picker read times out
- **WHEN** `ListMyVouchers` does not answer within 2 s while the payment step renders
- **THEN** the system SHALL render the picker with the platform vouchers only and no error banner within 2.5 s of the request
#### Scenario: W23 Anonymous visitor on an account page (permission)
- **WHEN** an anonymous visitor opens `/account/following`
- **THEN** the system SHALL redirect to `/login` without rendering the checklist or issuing any of its reads
