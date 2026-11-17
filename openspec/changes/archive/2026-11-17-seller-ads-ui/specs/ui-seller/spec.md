## ADDED Requirements

### Requirement: The seller ads page lists campaigns from the service

`/seller/ads` SHALL list the signed-in seller's ad campaigns from `SponsoredService.ListMyAdCampaigns`, 20 per page
with the page in the URL, each row showing the listing title, bid, budget, spent, remaining budget, impressions,
clicks and a status label, all money as integer VND. The list SHALL survive a reload and SHALL include a campaign
immediately after it is created. A failed list call SHALL affect only the list.

#### Scenario: Campaigns persist after reload

- **WHEN** a seller who created a campaign with budget 50 000 and bid 1 000 reloads `/seller/ads`
- **THEN** the table shows one row with "₫1.000" bid, "₫50.000" budget, "₫0" spent, "₫50.000" remaining, 0 impressions, 0 clicks and "Đang chạy"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Campaigns persist after reload`

#### Scenario: Spend and counters come from the service

- **WHEN** a buyer's click has charged the campaign once at bid 1 000 and the seller opens `/seller/ads`
- **THEN** the row shows "₫1.000" spent, "₫49.000" remaining, 1 click and at least 1 impression
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Spend and counters come from the service`

#### Scenario: Seller with no campaign sees an empty state

- **WHEN** a seller with no campaigns opens `/seller/ads`
- **THEN** the table shows "Chưa có chiến dịch nào" and the create form is still rendered
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `renders empty state`

#### Scenario: Campaigns are paged at 20

- **WHEN** a seller has 21 campaigns and opens `/seller/ads` and then `/seller/ads?page=2`
- **THEN** the first page shows 20 rows and the second shows 1 row
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/ads/page.test.tsx` `pages campaigns at 20`

#### Scenario: List failure degrades only the list

- **WHEN** `ListMyAdCampaigns` fails with UNAVAILABLE or exceeds the 2 s timeout
- **THEN** the list shows an error alert "Không tải được chiến dịch" with a "Thử lại" link, the create form renders, and the page responds 200
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/ads/page.test.tsx` `list error degrades only the list`

#### Scenario: Guest and non-seller are not authorized to open the page

- **WHEN** an unauthenticated guest (401) opens `/seller/ads`, and separately a signed-in buyer without `listing.write` opens it
- **THEN** the guest is redirected to the login page with `/seller/ads` as the return path, the buyer sees no campaign data, and no `ListMyAdCampaigns` call is made for either
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Guest and non-seller are not authorized to open the page`

### Requirement: A seller can pause and resume a campaign

Each campaign row SHALL offer "Tạm dừng" when the status is ACTIVE and "Tiếp tục" when it is PAUSED, calling
`PauseAdCampaign` or `ResumeAdCampaign` through a Server Action that returns `{ ok, error?, data? }`, shows a
pending state, is disabled while pending, toasts the outcome and refreshes the row from the service response. A
campaign in `PAUSED_BUDGET` SHALL show "Hết ngân sách" with the resume button disabled, and an ENDED campaign
SHALL show no action. A campaign of another seller SHALL NOT be actionable: the service returns NOT_FOUND and the
UI reports "Không tìm thấy chiến dịch.".

#### Scenario: Pause an active campaign

- **WHEN** the seller activates "Tạm dừng" on an ACTIVE campaign
- **THEN** the row status becomes "Tạm dừng" and the button becomes "Tiếp tục", still so after reload, and the campaign is no longer served to buyers
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Pause an active campaign`

#### Scenario: Resume a paused campaign

- **WHEN** the seller activates "Tiếp tục" on a PAUSED campaign whose remaining budget is at least its bid
- **THEN** the row status becomes "Đang chạy" and the button becomes "Tạm dừng", still so after reload
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Resume a paused campaign`

#### Scenario: Budget-exhausted campaign cannot resume

- **WHEN** a campaign has spent so that remaining budget (₫400) is below its bid (₫500 or more) and the seller opens `/seller/ads`
- **THEN** the row shows "Hết ngân sách" and the resume button is disabled, and if the action is still forced the error toast reads "Ngân sách còn lại thấp hơn giá thầu, không thể tiếp tục chạy." and the status is unchanged
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/actions.test.ts` `resume maps FAILED_PRECONDITION to the budget message`

#### Scenario: Ended campaign has no action

- **WHEN** a campaign has status ENDED
- **THEN** its row shows "Đã kết thúc" and neither "Tạm dừng" nor "Tiếp tục" is rendered
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `ended row has no action`

#### Scenario: Double click issues one call

- **WHEN** the seller double-clicks "Tạm dừng"
- **THEN** the button is disabled after the first click and exactly one `PauseAdCampaign` call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/AdCampaignTable.test.tsx` `double click pauses once`

#### Scenario: Another seller's campaign is not found

- **WHEN** seller B submits Pause or Resume with the id of seller A's campaign
- **THEN** the action returns `ok: false` with "Không tìm thấy chiến dịch.", A's campaign is unchanged, and B's list does not contain it
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Another seller's campaign is not found`

### Requirement: The campaign form enforces the minimum bid

The create form and `createAdCampaignAction` SHALL reject a per-click bid below 500 VND, and a budget below the
bid, with a field error and without calling the service from the client path; a bid of exactly 500 VND SHALL be
accepted. Server INVALID_ARGUMENT SHALL still be shown as the form error.

#### Scenario: Bid below minimum is rejected

- **WHEN** the seller submits a campaign with bid 499
- **THEN** the field error "Giá thầu tối thiểu là ₫500." is shown and no `CreateAdCampaign` call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerForms.test.tsx` `rejects bid below 500`

#### Scenario: Bid of exactly 500 is accepted

- **WHEN** the seller submits a campaign with bid 500 and budget 5 000
- **THEN** the campaign is created and appears in the list as "Đang chạy" with "₫500" bid
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/frontend/seller_ads_ui.feature` `Bid of exactly 500 is accepted`

#### Scenario: Ad form: budget below the bid is rejected

- **WHEN** the seller submits bid 1 000 and budget 800
- **THEN** the field error "Ngân sách phải lớn hơn hoặc bằng giá thầu." is shown and no call is made
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/SellerForms.test.tsx` `rejects budget below bid`
