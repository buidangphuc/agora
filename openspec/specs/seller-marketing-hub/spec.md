# seller-marketing-hub Specification

## Purpose
Defines the `/seller/marketing` hub in the seller app where a seller creates, lists and stops shop vouchers and flash-sale campaigns.

## Requirements

### Requirement: Marketing hub page and navigation

The seller app SHALL serve `/seller/marketing` to a signed-in principal with scope `listing.write`, with the tabs
"Voucher của shop" (`?tab=voucher`, default) and "Flash sale" (`?tab=flashsale`), and `SELLER_NAV` SHALL list it
as "Marketing".

#### Scenario: Seller opens the hub from the navigation
- **WHEN** a seller clicks "Marketing" in the seller navigation
- **THEN** the browser lands on `/seller/marketing` and the "Voucher của shop" tab is selected

#### Scenario: Tab is kept in the URL
- **WHEN** a seller selects the "Flash sale" tab and reloads the page
- **THEN** the URL contains `tab=flashsale` and the "Flash sale" tab is selected after reload

#### Scenario: Unknown tab value falls back to the default
- **WHEN** a seller opens `/seller/marketing?tab=bogus`
- **THEN** the "Voucher của shop" tab is shown and no error state is rendered
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/marketing/page.test.tsx` `falls back to the voucher tab for an unknown tab value`

#### Scenario: Buyer account is denied the hub
- **WHEN** a signed-in buyer without scope `listing.write` opens `/seller/marketing`
- **THEN** the seller onboarding screen ("Đồng ý và trở thành Người Bán") is shown instead of the hub and no promotion RPC is called

#### Scenario: Anonymous visitor is redirected to login
- **WHEN** an anonymous visitor opens `/seller/marketing`
- **THEN** the visitor is redirected to the login page

#### Scenario: One tab's dependency outage does not blank the other
- **WHEN** `ListMyCampaigns` fails with UNAVAILABLE while the seller opens the "Flash sale" tab
- **THEN** that tab shows a retry state, the navigation stays visible, and the "Voucher của shop" tab still lists vouchers
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/marketing/page.test.tsx` `keeps the other tab usable when ListMyCampaigns is unavailable`

### Requirement: Seller lists own shop vouchers

The "Voucher của shop" tab SHALL list only the caller's shop-scope vouchers of every state, fetched through
`VoucherService.ListMyShopVouchers` one cursor page (default 20) at a time, each row showing code, discount, used/quota
and a state label derived from `active`, `starts_at` and `ends_at`.

#### Scenario: Vouchers of every state are listed with used and quota
- **WHEN** a seller who owns one active, one scheduled, one ended and one deactivated shop voucher opens the tab
- **THEN** four rows are shown with the labels Đang chạy, Sắp diễn ra, Đã kết thúc and Đã tắt and each row shows `used/quota`

#### Scenario: Other sellers' vouchers are never shown
- **WHEN** seller A opens the tab while seller B owns a shop voucher and a platform voucher exists
- **THEN** no row for seller B's or the platform voucher is shown

#### Scenario: Empty list shows an empty state
- **WHEN** a seller with no shop vouchers opens the tab
- **THEN** an empty state with a "Tạo voucher" call to action is shown and no error is shown

#### Scenario: Paging boundary at 21 vouchers
- **WHEN** a seller with 21 shop vouchers opens the tab
- **THEN** 20 rows are shown with a next-page control, and the next page shows 1 row

#### Scenario: Principal without listing.write gets no vouchers
- **WHEN** a principal without scope `listing.write` requests the voucher tab data
- **THEN** access is denied and `ListMyShopVouchers` is not called
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/marketing/page.test.tsx` `does not call ListMyShopVouchers without listing.write`

#### Scenario: Unknown cursor shows the retry state
- **WHEN** a seller opens the tab with a malformed `cursor` in the URL and the RPC answers INVALID_ARGUMENT
- **THEN** the retry state is shown and no rows are shown
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/marketing/page.test.tsx` `shows the retry state for a malformed cursor`

### Requirement: Seller creates a shop voucher

The tab SHALL offer a create form that calls `VoucherService.CreateVoucher` with scope SHOP and never sends a seller id,
and SHALL show the new voucher in the list after success.

#### Scenario: Valid voucher is created and listed
- **WHEN** a seller submits code `GIAM10`, PERCENT 10, quota 100 and a window starting tomorrow
- **THEN** `CreateVoucher` is called once with scope SHOP and no seller id, and a row `GIAM10` with label Sắp diễn ra appears in the list within 2 s

#### Scenario: Empty code is rejected before any call
- **WHEN** a seller submits the form with an empty code
- **THEN** an inline error "Vui lòng nhập mã voucher" is shown and no RPC is called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/ShopVoucherForm.test.tsx` `rejects an empty code before any call`

#### Scenario: Window ending before it starts is rejected
- **WHEN** a seller submits an end time earlier than the start time
- **THEN** an inline error is shown and no RPC is called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/ShopVoucherForm.test.tsx` `rejects an end before the start before any call`

#### Scenario: Duplicate code shows the server error
- **WHEN** a seller submits a code that already exists and the RPC answers ALREADY_EXISTS
- **THEN** an error "Mã voucher đã tồn tại" is shown, the form keeps its values and the list is unchanged

#### Scenario: Seller without listing.write cannot submit
- **WHEN** a principal without scope `listing.write` posts the create-voucher server action
- **THEN** the action answers a denied result and `CreateVoucher` is not called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `marketing actions answer denied without listing.write`

### Requirement: Seller deactivates own shop voucher

Each non-deactivated row SHALL offer "Tắt voucher" that, after confirmation, calls `VoucherService.DeactivateMyShopVoucher`
with only the voucher id and then shows the row as Đã tắt.

#### Scenario: Deactivating an active voucher
- **WHEN** a seller confirms "Tắt voucher" on an active voucher
- **THEN** the row shows Đã tắt within 2 s and a buyer cannot apply that code at checkout preview

#### Scenario: Cancelling the confirmation changes nothing
- **WHEN** a seller opens the confirmation and chooses "Huỷ bỏ"
- **THEN** no RPC is called and the row state is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/VoucherTab.test.tsx` `cancelling the deactivate confirmation calls nothing`

#### Scenario: Deactivating twice is accepted
- **WHEN** a seller deactivates a voucher that is already deactivated, for example from a stale tab
- **THEN** the row shows Đã tắt and no error is shown
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `deactivating an already deactivated voucher is accepted`

#### Scenario: Another seller's voucher id is denied
- **WHEN** a seller posts the deactivate action with a voucher id owned by another seller
- **THEN** the RPC answers PERMISSION_DENIED, the message "Không có quyền hoặc voucher không tồn tại" is shown, and the voucher stays active

### Requirement: Seller lists own flash sales

The "Flash sale" tab SHALL list only the caller's campaigns through `FlashSaleService.ListMyCampaigns`, one cursor page
(default 20) at a time, with a status filter (Tất cả, Sắp diễn ra, Đang diễn ra, Đã kết thúc, Đã huỷ) sent as `statuses`,
each row showing listing title, sale price, status and sold/cap.

#### Scenario: Campaigns are listed with status and sold over cap
- **WHEN** a seller with a SCHEDULED campaign (cap 10) and an ACTIVE campaign (cap 20, sold 5) opens the tab
- **THEN** two rows show Sắp diễn ra with `0/10` and Đang diễn ra with `5/20`
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/promotion.test.ts` `maps campaign status and sold over cap`

#### Scenario: Hub status filter narrows the list
- **WHEN** a seller selects the filter "Đã huỷ"
- **THEN** the URL contains `status=cancelled`, `ListMyCampaigns` is called with `statuses=[CANCELLED]`, and only cancelled rows are shown

#### Scenario: Other sellers' campaigns are never shown
- **WHEN** seller A opens the tab while seller B owns a campaign
- **THEN** no row for seller B's campaign is shown

#### Scenario: Principal without listing.write gets no campaigns
- **WHEN** a principal without scope `listing.write` requests the flash sale tab data
- **THEN** access is denied and `ListMyCampaigns` is not called
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/marketing/page.test.tsx` `does not call ListMyCampaigns without listing.write`

#### Scenario: Empty list shows an empty state
- **WHEN** a seller with no campaigns opens the tab
- **THEN** an empty state with a "Tạo flash sale" call to action is shown and no error is shown

#### Scenario: Listing title lookup failure does not hide the row
- **WHEN** a campaign's listing title cannot be resolved because the listing service is unavailable
- **THEN** the row is still shown with the listing id in place of the title
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/FlashSaleTab.test.tsx` `keeps the row with the listing id when the title lookup fails`

### Requirement: Seller creates a flash sale for own listing

The tab SHALL offer a create form whose listing picker searches only the caller's PUBLISHED listings through
`ListingService.ListMyListings` (`statuses=[PUBLISHED]`, `query`, one page of at most 20 per search) and creates the campaign
with `FlashSaleService.CreateCampaign`, sending a per-form idempotency key and no seller id.

#### Scenario: Picker searches server-side
- **WHEN** a seller types "ao" in the listing picker
- **THEN** one `ListMyListings` call is made with `query="ao"` and `statuses=[PUBLISHED]`, no further pages are walked, and at most 20 options are shown

#### Scenario: Valid campaign is created and listed
- **WHEN** a seller picks a published listing and submits sale price 50000, cap 10 and a future window
- **THEN** `CreateCampaign` is called once and a SCHEDULED row appears in the list within 2 s

#### Scenario: Double submit creates one campaign
- **WHEN** a seller double-clicks the submit button
- **THEN** exactly one campaign exists afterwards
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/FlashSaleForm.test.tsx` `double submit sends one CreateCampaign with one idempotency key`

#### Scenario: Missing listing is rejected before any call
- **WHEN** a seller submits the form without picking a listing
- **THEN** an inline error is shown and no RPC is called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/FlashSaleForm.test.tsx` `rejects a missing listing before any call`

#### Scenario: Sale price of zero or cap below one is rejected
- **WHEN** a seller submits sale price 0 or cap 0
- **THEN** an inline error is shown and no RPC is called
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/FlashSaleForm.test.tsx` `rejects sale price 0 and cap 0 before any call`

#### Scenario: Server rejection is shown and nothing is created
- **WHEN** the RPC answers INVALID_ARGUMENT or FAILED_PRECONDITION, for example a sale price not below the listing price
- **THEN** the server message is shown, the form keeps its values and the list is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/marketing/FlashSaleForm.test.tsx` `shows the server message and keeps the form values`

#### Scenario: Foreign listing id is denied
- **WHEN** a seller posts the create-campaign action with a listing id owned by another seller
- **THEN** the RPC answers PERMISSION_DENIED, an error is shown, and no campaign is created
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `createCampaignAction maps PERMISSION_DENIED for a foreign listing`

### Requirement: Seller cancels a scheduled flash sale

Only rows with status SCHEDULED SHALL offer "Huỷ", which after confirmation calls `FlashSaleService.CancelMyCampaign`
with only the campaign id and then shows the row as Đã huỷ.

#### Scenario: Cancelling a scheduled campaign
- **WHEN** a seller confirms "Huỷ" on a SCHEDULED campaign
- **THEN** the row shows Đã huỷ within 2 s and the campaign never becomes active for buyers

#### Scenario: Active and ended campaigns offer no cancel control
- **WHEN** a seller views an ACTIVE, an ENDED and a CANCELLED campaign row
- **THEN** none of the three rows has a "Huỷ" control

#### Scenario: Cancelling a campaign that started meanwhile is refused
- **WHEN** a seller confirms "Huỷ" on a row that became ACTIVE since the page loaded and the RPC answers FAILED_PRECONDITION
- **THEN** the message "Flash sale đã bắt đầu, không thể huỷ" is shown and the row refreshes to Đang diễn ra

#### Scenario: Another seller's campaign id is denied
- **WHEN** a seller posts the cancel action with a campaign id owned by another seller
- **THEN** the RPC answers PERMISSION_DENIED, the message "Không có quyền hoặc flash sale không tồn tại" is shown, and the campaign is unchanged

### Requirement: Buyer voucher page has no create form

The buyer app `/vouchers` page SHALL NOT render a voucher create form for any principal, while still rendering the
voucher catalogue and the buyer's own vouchers.

#### Scenario: Seller visiting the buyer voucher page sees no create form
- **WHEN** a signed-in seller with scope `listing.write` opens `/vouchers` on the buyer app
- **THEN** the page shows the voucher catalogue and no create-voucher form

#### Scenario: Buyer voucher wallet still renders
- **WHEN** a buyer who holds a voucher opens `/vouchers`
- **THEN** the voucher appears in the buyer's vouchers section

#### Scenario: Create form is not rendered even when the page cannot load vouchers
- **WHEN** `ListVouchers` fails with UNAVAILABLE and a seller opens `/vouchers`
- **THEN** the page shows its empty catalogue state and no create-voucher form
- **VERIFIED BY**: `team-frontend/src/app/(shop)/vouchers/page.test.tsx` `renders no create form when ListVouchers is unavailable`
