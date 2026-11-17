# seller-getting-started Specification

## Purpose
Defines the "Bắt đầu bán hàng" checklist card on the seller dashboard that shows which first-sale steps a new seller has done and links to each.

## Requirements

### Requirement: The seller dashboard shows a first-steps checklist

`/seller` SHALL render a card titled "Bắt đầu bán hàng" for a principal with `listing.write`, listing four steps in this order: "Đặt tên gian hàng", "Thêm ảnh bìa gian hàng", "Đăng sản phẩm đầu tiên", "Tạo voucher hoặc flash sale đầu tiên". Each step SHALL show one state (done, not done, or "chưa rõ") and a link to its page: `/seller/shop`, `/seller/shop`, `/seller/new`, `/seller/marketing`. Step state SHALL be derived on every render from the seller's own data: display name non-empty (`GetStorefront`), banner URL non-empty (`GetStorefront`), at least one listing of any status (`ListMyListings`, page size 1), at least one shop voucher or flash-sale campaign of any state (`ListMyShopVouchers`, `ListMyCampaigns`, page size 1). The card SHALL contain no KYC step. The seller id SHALL come only from the session principal.

#### Scenario: Getting started: a new seller sees all four steps not done
- **WHEN** a seller with no display name, no banner, no listing, no voucher and no campaign opens `/seller`
- **THEN** the card "Bắt đầu bán hàng" shows exactly four steps, all not done, with links `/seller/shop`, `/seller/shop`, `/seller/new`, `/seller/marketing`, and no step mentions KYC
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `all steps not done for an empty shop`

#### Scenario: Getting started: done steps are marked from the shop data
- **WHEN** the seller has display name "Shop An", an empty banner URL, one DRAFT listing, no voucher and one SCHEDULED campaign
- **THEN** steps 1, 3 and 4 are done, step 2 is not done, and the card is still shown
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `marks steps from storefront, listing and promotion data`

#### Scenario: Getting started: a voucher alone completes the marketing step
- **WHEN** the seller has one deactivated shop voucher and no campaign
- **THEN** step 4 is done
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `a voucher of any state completes step 4`

#### Scenario: Getting started: a seller without a storefront row sees steps 1 and 2 not done
- **WHEN** `GetStorefront` answers NOT_FOUND for the seller
- **THEN** steps 1 and 2 are not done (not "chưa rõ") and steps 3 and 4 are evaluated from their own sources
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `missing storefront is not done not unknown`

#### Scenario: Getting started: the seller id comes from the session, not the request
- **WHEN** `/seller` is requested with the query string `?sellerId=s-other`
- **THEN** `GetStorefront` is called with the principal's seller id and never with `s-other`
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/(dashboard)/page.test.tsx` `getting started uses the principal seller id`

#### Scenario: Getting started: a visitor without listing.write never reaches the card
- **WHEN** `/seller` renders for an anonymous visitor, and again for a signed-in principal without `listing.write`
- **THEN** the anonymous visitor is redirected to login, the other principal gets no card, and none of the four data sources is called in either case
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/(dashboard)/page.test.tsx` `no card and no calls without seller scope`

### Requirement: The card hides when every step is done and never blocks the dashboard

The card SHALL not render when all four steps are done. A step whose data source fails or exceeds 3 s SHALL show "chưa rõ" with its link, and SHALL count as not done for hiding. Step 4 SHALL be "chưa rõ" only when no source returned a row and at least one source failed. The card SHALL render in its own `Suspense` boundary so the KPI row, recent orders and product list render without waiting for it, and a failure of any source SHALL NOT change the dashboard HTTP status or hide other sections.

#### Scenario: Getting started: the card hides once all steps are done
- **WHEN** the seller has a display name, a banner, one listing and one voucher
- **THEN** the card "Bắt đầu bán hàng" is absent from `/seller` and the KPI row is present
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/GettingStartedCard.test.tsx` `renders nothing when all steps are done`

#### Scenario: Getting started: an unavailable source shows chưa rõ and keeps the dashboard
- **WHEN** `ListMyListings` answers UNAVAILABLE while the other three sources answer normally
- **THEN** step 3 shows "chưa rõ" with its link `/seller/new`, the card stays visible even if the other steps are done, and the dashboard still renders its KPI row, recent orders and product list
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/GettingStartedCard.test.tsx` `failed source shows chua ro and card stays`

#### Scenario: Getting started: one promotion source down and the other empty is unknown
- **WHEN** `ListMyShopVouchers` answers UNAVAILABLE and `ListMyCampaigns` returns no rows
- **THEN** step 4 shows "chưa rõ"
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `step 4 unknown when one source fails and the other is empty`

#### Scenario: Getting started: one promotion source down and the other has a row is done
- **WHEN** `ListMyShopVouchers` answers UNAVAILABLE and `ListMyCampaigns` returns one campaign
- **THEN** step 4 is done
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `step 4 done when one source has a row though the other fails`

#### Scenario: Getting started: a source slower than 3 s times out
- **WHEN** `GetStorefront` does not answer within 3 s
- **THEN** steps 1 and 2 show "chưa rõ" at 3 s and the page response is not delayed beyond the other sections
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/gettingStarted.test.ts` `a source slower than 3s becomes unknown`

#### Scenario: Getting started: exactly one step missing keeps the card
- **WHEN** three steps are done and only the banner is missing
- **THEN** the card is shown with step 2 not done and the other three done
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/GettingStartedCard.test.tsx` `one missing step keeps the card`

### Requirement: The checklist is verified in the browser against the real services

The checklist SHALL reflect real shop state: completing a step through its linked page SHALL turn it done on the next visit to `/seller`, with the card hidden once all four are done.

#### Scenario: Getting started: a new seller completes the steps in the browser
- **WHEN** a freshly granted seller opens `/seller`, saves a shop name and a banner on `/seller/shop`, creates a listing on `/seller/new`, creates a shop voucher on `/seller/marketing`, and returns to `/seller`
- **THEN** the first visit shows four not-done steps, each completed step is done on the next visit, and after the last step the card "Bắt đầu bán hàng" is absent
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_getting_started.feature` `Getting started: a new seller completes the steps in the browser`

#### Scenario: Getting started: a seller with a flash sale only sees the marketing step done
- **WHEN** a seller whose only activity is one flash-sale campaign on their own listing opens `/seller`
- **THEN** step 3 and step 4 are done, steps 1 and 2 are not done, and the card is shown
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_getting_started.feature` `Getting started: a seller with a flash sale only sees the marketing step done`

#### Scenario: Getting started: a buyer without the seller role does not see the card
- **WHEN** a buyer without `listing.write` opens `/seller` on the seller app
- **THEN** the page shows the onboarding button "Đồng ý và trở thành Người Bán" and no text "Bắt đầu bán hàng"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_getting_started.feature` `Getting started: a buyer without the seller role does not see the card`
