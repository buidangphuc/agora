## ADDED Requirements

### Requirement: A seller lists their own flash-sale campaigns

`FlashSaleService.ListMyCampaigns` SHALL return only campaigns whose `seller_id` equals the calling principal id,
newest start first, with the derived `status` (SCHEDULED, ACTIVE, ENDED, CANCELLED), `stock_sold` and `stock_cap`,
filtered by `statuses` when given and paged with the offset cursor (default 20, max 200). The request SHALL carry no
seller field. The caller SHALL hold scope `listing.write`; otherwise the call is PERMISSION_DENIED, and an anonymous
caller is UNAUTHENTICATED. Every `CreateCampaign` SHALL store the listing owner as `seller_id`.

#### Scenario: Seller sees only their own campaigns with status
- **WHEN** seller A, who created one SCHEDULED and one ACTIVE campaign, calls ListMyCampaigns while seller B also has a campaign
- **THEN** the response holds exactly A's two campaigns, each with `seller_id` = A, the right status, and `stock_cap`/`stock_sold`, and none of B's
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller lists only their own campaigns"

#### Scenario: Status filter narrows the list
- **WHEN** seller A calls ListMyCampaigns with `statuses` = [SCHEDULED]
- **THEN** only A's SCHEDULED campaigns are returned
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "The campaign list can be filtered by status"

#### Scenario: Paging boundary at 21 campaigns
- **WHEN** seller A, who has 21 campaigns, calls ListMyCampaigns with `page_size` 20 and then with the returned cursor
- **THEN** the first call returns 20 campaigns and a non-empty `next_cursor`, the second returns 1 campaign and an empty `next_cursor`, with no repeats
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestListMyCampaignsPaging_PagingBoundaryAt21Campaigns`

#### Scenario: Caller without listing.write is denied
- **WHEN** a buyer without scope `listing.write` calls ListMyCampaigns through the gateway
- **THEN** the call fails with PERMISSION_DENIED and no campaign data is returned
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A buyer cannot use the seller campaign RPCs"

#### Scenario: Anonymous caller is rejected
- **WHEN** an anonymous caller calls ListMyCampaigns through the gateway
- **THEN** the call fails with UNAUTHENTICATED
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyCampaignsAnonymous`

#### Scenario: A new campaign records the listing owner
- **WHEN** seller A creates a campaign for A's listing, and an admin creates one for seller B's listing
- **THEN** A's campaign has `seller_id` = A and the admin-created campaign has `seller_id` = B, and the admin campaign appears in B's ListMyCampaigns
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCampaignSellerCreateStoresListingOwner_NewCampaignRecordsListingOwner`

### Requirement: A seller cancels only their own SCHEDULED campaign

`FlashSaleService.CancelMyCampaign` SHALL cancel the campaign with `campaign_id` only when its `seller_id` equals
the calling principal id and its status is SCHEDULED, returning the campaign with status CANCELLED and
`cancelled_at`. A foreign, unknown, empty-`seller_id` or malformed id SHALL return PERMISSION_DENIED with the same
message. A campaign that is ACTIVE or ENDED SHALL return FAILED_PRECONDITION and stay unchanged. A campaign that is
already CANCELLED SHALL return OK unchanged (idempotent). The status check and the write SHALL be one locked
transaction, and no audit event SHALL be written.

#### Scenario: Seller cancels their SCHEDULED campaign
- **WHEN** seller A cancels A's SCHEDULED campaign
- **THEN** the response has status CANCELLED and a `cancelled_at`, ListMyCampaigns shows it CANCELLED, and the listing's flash price is no longer active
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller cancels their scheduled campaign"

#### Scenario: Cancelling an ACTIVE campaign is refused
- **WHEN** seller A cancels A's ACTIVE campaign
- **THEN** the call fails with FAILED_PRECONDITION and the campaign stays ACTIVE with `cancelled_at` unset
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller cannot cancel an active campaign"

#### Scenario: Cancelling another seller's campaign is denied like an unknown id
- **WHEN** seller B cancels seller A's campaign, and separately cancels a non-existent campaign id
- **THEN** both calls fail with PERMISSION_DENIED with an identical message, and A's campaign keeps its status
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller cannot cancel another seller's campaign"

#### Scenario: Cancelling twice is idempotent
- **WHEN** seller A cancels the same SCHEDULED campaign twice
- **THEN** both calls return OK with status CANCELLED and the same `cancelled_at`
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCancelMyCampaignIdempotent_CancellingTwiceIsIdempotent`

#### Scenario: Campaign starting during the cancel is not cancelled
- **WHEN** a campaign's `starts_at` passes between the seller's page load and the CancelMyCampaign call
- **THEN** the call fails with FAILED_PRECONDITION because the status is evaluated under the row lock at call time
- **VERIFIED BY**: team-promotion `internal/store` `TestCancelIfScheduledOwnedBoundary_CampaignStartingDuringTheCancelIsNotCancelled`

#### Scenario: Seller cancellation writes no audit event
- **WHEN** a seller cancels their SCHEDULED campaign while team-audit is stopped
- **THEN** the call returns OK and QueryAuditLog (admin) returns zero `campaign.cancel` events for that campaign id
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "Seller cancellation does not depend on audit"

#### Scenario: Admin cancel keeps its admin rules
- **WHEN** an admin calls the existing CancelCampaign on a SCHEDULED campaign of any seller
- **THEN** it is cancelled and one `campaign.cancel` audit event is written, as before this change
- **VERIFIED BY**: platform-e2e `features/promo/flash_sale_admin.feature` existing cancel scenario (unchanged)

### Requirement: A seller lists their own shop vouchers

`VoucherService.ListMyShopVouchers` SHALL return the vouchers with scope SHOP and `seller_id` equal to the calling
principal id, in every state (active, scheduled, ended by date or quota, deactivated), newest first, with `quota`,
`used`, `starts_at`, `ends_at`, `active` and `deactivated_at`, paged with the offset cursor (default 20, max 200).
User-bound vouchers and platform vouchers SHALL NOT appear. The request SHALL carry no seller field. The caller
SHALL hold scope `listing.write`; otherwise PERMISSION_DENIED, and anonymous is UNAUTHENTICATED.

#### Scenario: Seller sees own vouchers of every state
- **WHEN** seller A, who has an active, a deactivated and an expired shop voucher, calls ListMyShopVouchers while seller B also has a shop voucher and a platform voucher exists
- **THEN** the response holds exactly A's three vouchers with `used`/`quota` set, and neither B's nor the platform voucher
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller lists their shop vouchers in every state"

#### Scenario: Empty list for a seller with no vouchers
- **WHEN** a seller with no shop voucher calls ListMyShopVouchers
- **THEN** the response has zero vouchers and an empty `next_cursor`
- **VERIFIED BY**: team-promotion `internal/voucher` `TestListMyShopVouchersEmpty`

#### Scenario: Buyer without listing.write is denied
- **WHEN** a buyer without scope `listing.write` calls ListMyShopVouchers through the gateway
- **THEN** the call fails with PERMISSION_DENIED
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A buyer cannot use the seller voucher RPCs"

#### Scenario: Page size above the maximum is capped
- **WHEN** seller A calls ListMyShopVouchers with `page_size` 1000
- **THEN** at most 200 vouchers are returned
- **VERIFIED BY**: team-promotion `internal/voucher` `TestListMyShopVouchersPageCap`

### Requirement: A seller deactivates only their own shop voucher

`VoucherService.DeactivateMyShopVoucher` SHALL set `active=false` and keep the first `deactivated_at` for the
voucher with `voucher_id` only when it has scope SHOP and `seller_id` equal to the calling principal id, returning
the voucher. It SHALL be idempotent. A foreign, unknown, platform-scope, user-bound or empty id SHALL return
PERMISSION_DENIED with the same message. No audit event SHALL be written. A deactivated voucher SHALL be refused by
`ValidateAndReserve`, as for the admin path.

#### Scenario: Seller deactivates their voucher
- **WHEN** seller A deactivates A's active shop voucher
- **THEN** the response has `active` = false and a `deactivated_at`, and a buyer applying its code at checkout preview gets `valid` = false
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller deactivates their shop voucher"

#### Scenario: Deactivating twice keeps the first timestamp
- **WHEN** seller A deactivates the same voucher twice
- **THEN** both calls return OK and `deactivated_at` is identical in both
- **VERIFIED BY**: team-promotion `internal/voucher` `TestDeactivateMyShopVoucherIdempotent`

#### Scenario: Deactivating another seller's voucher is denied like an unknown id
- **WHEN** seller B deactivates seller A's shop voucher, and separately a non-existent voucher id
- **THEN** both calls fail with PERMISSION_DENIED with an identical message, and A's voucher stays active
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "A seller cannot deactivate another seller's voucher"

#### Scenario: A platform voucher cannot be deactivated by a seller
- **WHEN** a seller deactivates a platform-scope voucher by its id
- **THEN** the call fails with PERMISSION_DENIED and the voucher stays active
- **VERIFIED BY**: team-promotion `internal/rpc` `TestDeactivateMyShopVoucherPlatformScopeDenied`

#### Scenario: Seller deactivation writes no audit event
- **WHEN** a seller deactivates their shop voucher while team-audit is stopped
- **THEN** the call returns OK and QueryAuditLog (admin) returns zero `voucher.deactivate` events for its code
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "Seller deactivation does not depend on audit"

#### Scenario: Admin deactivate keeps its admin rules
- **WHEN** an admin calls the existing DeactivateVoucher
- **THEN** one `voucher.deactivate` audit event is written first and the voucher is deactivated, as before this change
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_vouchers.feature` existing deactivate scenario (unchanged)

### Requirement: The gateway exposes the seller RPCs without opening the admin RPCs

team-gateway SHALL forward `ListMyCampaigns`, `CancelMyCampaign`, `ListMyShopVouchers` and
`DeactivateMyShopVoucher` to team-promotion with the caller's principal, and SHALL NOT add them to `adminProcedures`.
`ListCampaigns`, `CancelCampaign`, `UpdateCampaign`, `DeactivateVoucher` and `UpdateVoucher` SHALL keep requiring
`promotion.admin` at the edge. IF team-promotion is down, THEN the forwarded call SHALL fail with UNAVAILABLE and
ErrorInfo reason `PROMOTION_UNAVAILABLE`.

#### Scenario: Seller reaches the seller RPCs but not the admin ones
- **WHEN** a seller (listing.write, no promotion.admin) calls ListMyCampaigns and then the admin ListCampaigns through the gateway
- **THEN** ListMyCampaigns returns OK and ListCampaigns fails with PERMISSION_DENIED
- **VERIFIED BY**: platform-e2e `features/promo/promotion_seller_self_service.feature` "The admin RPCs stay closed to sellers"

#### Scenario: Promotion down surfaces as unavailable
- **WHEN** team-promotion is stopped and a seller calls ListMyShopVouchers through the gateway
- **THEN** the call fails with UNAVAILABLE and ErrorInfo reason `PROMOTION_UNAVAILABLE`
- **VERIFIED BY**: team-gateway `internal/edge` `TestPromotionSellerRPCsUnavailable`

### Requirement: Existing campaigns are backfilled with their seller

`team-promotion backfill-campaign-sellers` SHALL set `seller_id` from `ListingService.GetListing(listing_id).seller_id`
on every campaign whose `seller_id` is `''`, in batches of 100, and SHALL be idempotent: a second run changes no row.
A row whose listing is not found SHALL stay `''` and be counted in the summary; a lookup failure SHALL stop the run
with a non-zero exit, leaving finished batches intact. The migration SHALL add the column with default `''` so old
rows keep working for every existing RPC.

#### Scenario: Backfill fills old campaigns once
- **WHEN** three campaigns with `seller_id` = '' exist for listings owned by A, A and B, and the subcommand runs twice
- **THEN** after the first run their `seller_id` is A, A and B and the summary reports 3 updated; the second run reports 0 updated and changes no row
- **VERIFIED BY**: team-promotion `internal/app` `TestBackfillCampaignSellersIdempotent`

#### Scenario: Deleted listing is left unassigned
- **WHEN** a campaign's listing no longer exists in team-domain and the backfill runs
- **THEN** that campaign keeps `seller_id` = '' and the summary counts it as skipped, and no seller's ListMyCampaigns returns it
- **VERIFIED BY**: team-promotion `internal/app` `TestBackfillCampaignSellersSkipsMissingListing`

#### Scenario: Team-domain down stops the backfill
- **WHEN** team-domain is unavailable during the backfill
- **THEN** the subcommand exits non-zero without changing the current batch, and a re-run after recovery completes
- **VERIFIED BY**: team-promotion `internal/app` `TestBackfillCampaignSellersStopsOnLookupFailure`

#### Scenario: Migration keeps old rows valid
- **WHEN** migration 0009 is applied to a database holding campaigns created before it
- **THEN** those rows have `seller_id` = '' and the admin ListCampaigns, GetActiveFlashSale and stock reserve return the same results as before
- **VERIFIED BY**: team-promotion `internal/store` `TestMigration0009CampaignSeller_KeepsOldRowsValid`
