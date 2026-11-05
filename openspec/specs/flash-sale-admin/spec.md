# flash-sale-admin Specification

## Purpose
TBD - created by archiving change promotion-admin-campaigns. Update Purpose after archive.

## Requirements

### Requirement: Admin lists all campaigns with status filter

`FlashSaleService.ListCampaigns` called by a caller holding scope `admin` SHALL return campaigns whose derived status is in `statuses` (all campaigns when empty), ordered by `starts_at` descending with ties broken by id, paged by `page` (default 20, max 200), each carrying `status` and `cancelled_at`. Status SHALL be CANCELLED when `cancelled_at` is set, else SCHEDULED before `starts_at`, ACTIVE from `starts_at` until `ends_at`, ENDED after. The response SHALL set `flash_sale_disabled=true` when the flash-sale kill-switch is OFF, and still return the campaigns. `ListActiveCampaigns` SHALL set `flash_sale_disabled=true` with an empty list when the kill-switch is OFF. A caller without scope `admin` SHALL be refused.

#### Scenario: List filters by status
- **WHEN** an admin calls ListCampaigns with statuses [SCHEDULED] while one scheduled, one active, one ended and one cancelled campaign exist
- **THEN** the response contains exactly the scheduled campaign with `status=CAMPAIGN_STATUS_SCHEDULED`
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Admin filters campaigns by status"

#### Scenario: List with no filter, newest first
- **WHEN** an admin calls ListCampaigns with empty statuses and campaigns start at T1 < T2 < T3
- **THEN** the response lists T3, T2, T1 in that order, each with a non-UNSPECIFIED status
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestListCampaignsNewestFirstAllStatuses`

#### Scenario: Paging boundary
- **WHEN** an admin calls ListCampaigns with page_size 2 over 3 campaigns and then with the returned cursor
- **THEN** the first page has 2 campaigns and a cursor, the second has 1 campaign and an empty cursor
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestListCampaignsPaging`

#### Scenario: Status derivation at window edges
- **WHEN** the clock equals a campaign's `starts_at`, and later equals its `ends_at`
- **THEN** the status is ACTIVE at `starts_at` and ENDED at `ends_at`
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCampaignStatusBoundaries`

#### Scenario: Kill-switch off is reported
- **WHEN** the flash-sale kill-switch is OFF and an admin calls ListCampaigns, and anyone calls ListActiveCampaigns
- **THEN** ListCampaigns returns the campaigns with `flash_sale_disabled=true` and ListActiveCampaigns returns zero campaigns with `flash_sale_disabled=true`
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Kill-switch off is reported by both list calls"

#### Scenario: Non-admin cannot list all campaigns
- **WHEN** a seller or an anonymous caller calls ListCampaigns through the gateway
- **THEN** the call is refused PERMISSION_DENIED for the seller and UNAUTHENTICATED for the anonymous caller, and no campaign is returned
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Only an admin lists campaigns"

#### Scenario: Page size clamp
- **WHEN** an admin calls ListCampaigns with page_size 500 over 250 campaigns
- **THEN** the response contains 200 campaigns and a cursor
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestListCampaignsPageSizeClamp`

### Requirement: Admin cancels a campaign without losing sales

`FlashSaleService.CancelCampaign` called by an admin SHALL write audit entry `campaign.cancel` (target_type `flash_sale`, target_id campaign id) first, then set `cancelled_at` and return the campaign with status CANCELLED. After the call, `ReserveFlashSaleStock` for that campaign SHALL return not reserved with reason `not_active`. Holds that exist before the cancel SHALL still commit or release normally and `stock_sold` SHALL be unchanged. Cancelling a cancelled campaign SHALL be a no-op returning the same `cancelled_at`; cancelling an ended campaign SHALL return FAILED_PRECONDITION; an unknown id SHALL return NOT_FOUND. IF the audit write fails, THEN the call SHALL return UNAVAILABLE and the campaign SHALL stay unchanged.

#### Scenario: Cancel stops new holds, keeps existing holds and sold
- **WHEN** an admin cancels an active campaign that has 2 units sold and 1 unit held, then a buyer tries to reserve 1 unit, then the held unit commits
- **THEN** the new reserve returns reserved=false reason `not_active`, the commit returns OK, and the campaign shows stock_sold 3
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Cancelling an active campaign keeps held and sold units"

#### Scenario: Cancel is audited before the write
- **WHEN** an admin cancels a campaign
- **THEN** the recorded call order is audit write, then store update, and QueryAuditLog returns one `campaign.cancel` event with the admin as actor
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCancelCampaignAuditBeforeWrite`

#### Scenario: Cancel while audit is down
- **WHEN** team-audit is stopped and an admin cancels an active campaign
- **THEN** the call returns UNAVAILABLE and ListCampaigns shows the campaign still ACTIVE
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Cancel is refused while audit is down"

#### Scenario: Cancel twice is a no-op
- **WHEN** an admin cancels the same campaign twice
- **THEN** both calls return CANCELLED with an identical `cancelled_at`
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCancelCampaignIdempotent`

#### Scenario: Cancel an ended or unknown campaign
- **WHEN** an admin cancels an ended campaign, and then an unknown campaign id
- **THEN** the first call returns FAILED_PRECONDITION and the second returns NOT_FOUND
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCancelCampaignErrors`

#### Scenario: Seller cannot cancel
- **WHEN** a seller calls CancelCampaign for their own listing's campaign
- **THEN** the call is refused with permission denied (PERMISSION_DENIED) and the campaign stays ACTIVE
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCancelCampaignRequiresAdmin`

#### Scenario: Anonymous cannot cancel
- **WHEN** an anonymous caller calls CancelCampaign through the gateway
- **THEN** the call is refused UNAUTHENTICATED and the campaign is unchanged
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Anonymous callers cannot cancel or edit"

#### Scenario: Cancel audit outage identification
- **WHEN** team-audit is down and an admin cancels a campaign
- **THEN** the call returns UNAVAILABLE with ErrorInfo domain `promotion.platform` and reason `AUDIT_UNAVAILABLE`
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCampaignWritesAuditDownErrorInfo`

#### Scenario: Reserve racing a cancel
- **WHEN** ReserveFlashSaleStock runs concurrently with CancelCampaign on one campaign
- **THEN** every reserve that returns after the cancel call returned is not reserved with reason `not_active`, and the number of holds never exceeds the stock cap
- **VERIFIED BY**: team-promotion `internal/store` `TestCancelRacesReserve`

### Requirement: Admin edits a campaign only while scheduled

`FlashSaleService.UpdateCampaign` called by an admin SHALL, for a campaign in status SCHEDULED, apply the same validation as create, write audit entry `campaign.update` first, then replace `sale_price`, `stock_cap`, `starts_at`, `ends_at` and return the campaign. For a campaign in any other status it SHALL return FAILED_PRECONDITION and change nothing. Validation failures SHALL return INVALID_ARGUMENT naming the field; unknown id NOT_FOUND; audit failure UNAVAILABLE with ErrorInfo domain `promotion.platform` reason `AUDIT_UNAVAILABLE` and no change (the same applies to cancel and create); LISTING_UNAVAILABLE UNAVAILABLE with no change.

#### Scenario: Edit a scheduled campaign
- **WHEN** an admin updates a scheduled campaign to sale_price 90000, stock_cap 50 and a later window
- **THEN** the response and a following ListCampaigns show those values, status SCHEDULED, and one `campaign.update` audit event exists
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Admin edits a scheduled campaign"

#### Scenario: Edit a started campaign is refused
- **WHEN** an admin updates an active, an ended and a cancelled campaign
- **THEN** each call returns FAILED_PRECONDITION and the stored values are unchanged
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Editing a started campaign is refused"

#### Scenario: Edit validates like create
- **WHEN** an admin updates a scheduled campaign with sale_price at or above the listing price
- **THEN** the call returns INVALID_ARGUMENT naming `sale_price` and the stored values are unchanged
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestUpdateCampaignValidation`

#### Scenario: Edit is audited before the write and fails closed
- **WHEN** an admin updates a scheduled campaign, with team-audit up and then down
- **THEN** with audit up the order is audit write then store update; with audit down the call returns UNAVAILABLE and the stored values are unchanged
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestUpdateCampaignAuditBeforeWriteFailsClosed`

#### Scenario: Seller cannot edit
- **WHEN** a seller calls UpdateCampaign on a scheduled campaign
- **THEN** the call is refused with permission denied (PERMISSION_DENIED) and the campaign is unchanged
- **VERIFIED BY**: team-promotion `internal/rpc` `TestUpdateCampaignRequiresAdmin`

#### Scenario: Update unknown campaign
- **WHEN** an admin updates a campaign id that does not exist
- **THEN** the call returns NOT_FOUND and no audit entry is written
- **VERIFIED BY**: team-promotion `internal/rpc` `TestUpdateCampaignNotFound`

#### Scenario: Update with listing service down
- **WHEN** team-listing is stopped and an admin updates a scheduled campaign
- **THEN** the call returns UNAVAILABLE with ErrorInfo domain `promotion.platform` and reason `LISTING_UNAVAILABLE`, and the stored values are unchanged
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestUpdateCampaignListingDown`

#### Scenario: Campaign starts between status check and write
- **WHEN** a campaign's `starts_at` passes between the SCHEDULED check and the update write of UpdateCampaign
- **THEN** the update is refused FAILED_PRECONDITION and the stored values are unchanged, because the write is conditional on the campaign still being scheduled
- **VERIFIED BY**: team-promotion `internal/store` `TestUpdateCampaignStartRace`

#### Scenario: Anonymous cannot edit
- **WHEN** an anonymous caller calls UpdateCampaign through the gateway
- **THEN** the call is refused UNAUTHENTICATED and the campaign is unchanged
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Anonymous callers cannot cancel or edit"

### Requirement: Campaign creation is validated server-side

`FlashSaleService.CreateCampaign` on both the seller and admin paths SHALL reject, before any write or audit, with INVALID_ARGUMENT naming the field: `sale_price` <= 0 or not below the listing price read through `ListingService.GetListing`; `stock_cap` <= 0; `ends_at` not after `starts_at`; `ends_at` not in the future; unknown listing (`listing_id`). IF `ListingService.GetListing` is unreachable or times out, THEN the call SHALL return UNAVAILABLE with ErrorInfo reason `LISTING_UNAVAILABLE` and create nothing. The seller path SHALL keep the `listing.Owner` check, ignore any idempotency_key and create no admin idempotency record; an unknown listing on the seller path SHALL return INVALID_ARGUMENT naming `listing_id`. LISTING_UNAVAILABLE carries ErrorInfo domain `promotion.platform`.

#### Scenario: Price not below listing price
- **WHEN** an admin creates a campaign with sale_price equal to the listing price
- **THEN** the call returns INVALID_ARGUMENT naming `sale_price` and no campaign or audit entry exists
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Sale price at or above listing price is refused"

#### Scenario: Seller path is validated too
- **WHEN** a seller creates a campaign for their own listing with sale_price above the listing price
- **THEN** the call returns INVALID_ARGUMENT naming `sale_price`
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignSellerPathValidatesPrice`

#### Scenario: Bad stock cap and window
- **WHEN** an admin creates campaigns with stock_cap 0, with ends_at equal to starts_at, and with ends_at in the past
- **THEN** each returns INVALID_ARGUMENT naming `stock_cap`, `ends_at` and `ends_at` respectively
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignFieldValidation`

#### Scenario: Boundary values accepted
- **WHEN** an admin creates a campaign with sale_price one below the listing price, stock_cap 1, and ends_at one second after starts_at in the future
- **THEN** the call returns OK with status SCHEDULED
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignBoundaryAccepted`

#### Scenario: Unknown listing
- **WHEN** an admin creates a campaign for a listing id that does not exist
- **THEN** the call returns INVALID_ARGUMENT naming `listing_id`
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Unknown listing is refused"

#### Scenario: Listing service down
- **WHEN** team-listing is stopped and an admin creates a campaign
- **THEN** the call returns UNAVAILABLE with ErrorInfo reason `LISTING_UNAVAILABLE` within 2.5 s and ListCampaigns shows no new campaign
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Listing outage blocks campaign creation"

#### Scenario: Seller of another listing is denied
- **WHEN** a seller creates a campaign for a listing owned by someone else
- **THEN** the call returns PERMISSION_DENIED and nothing is created
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateCampaignSellerOwnershipDenied`

#### Scenario: Seller path unknown listing
- **WHEN** a seller creates a campaign for a listing id that does not exist
- **THEN** the call returns INVALID_ARGUMENT naming `listing_id`, and nothing is created
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignSellerUnknownListing`

#### Scenario: Create with audit down
- **WHEN** team-audit is stopped and an admin creates a valid campaign
- **THEN** the call returns UNAVAILABLE with ErrorInfo domain `promotion.platform` and reason `AUDIT_UNAVAILABLE`, no campaign exists, and no idempotency record is stored so a retry with the same key can succeed
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Create is refused while audit is down"

### Requirement: Campaign creation is idempotent per admin

`FlashSaleService.CreateCampaign` on the admin path SHALL require `idempotency_key` of 8 to 64 characters `[A-Za-z0-9_-]`, else INVALID_ARGUMENT naming `idempotency_key`. The pair (admin principal id, key) SHALL be stored with a SHA-256 hash of the payload (listing_id, variant_id, sale_price, stock_cap, starts_at, ends_at) for 24 h. The lookup SHALL run before validation and the listing lookup; a repeat with the same hash SHALL return the campaign's current stored state (for example CANCELLED or ENDED) and write no new audit entry, row or event. A repeat with a different hash SHALL return ALREADY_EXISTS and create nothing. The same key from a different admin SHALL be independent. After 24 h the key SHALL be reusable.

#### Scenario: Retry after timeout returns same campaign
- **WHEN** an admin sends CreateCampaign with key "retry-key-0001" twice with the same payload
- **THEN** both calls return the same campaign id and ListCampaigns shows exactly one such campaign
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Retrying create with the same key returns one campaign"

#### Scenario: Same key different payload
- **WHEN** an admin reuses key "retry-key-0001" with a different sale_price
- **THEN** the call returns ALREADY_EXISTS and the original campaign is unchanged
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Same key with a different payload is refused"

#### Scenario: Missing or malformed key
- **WHEN** an admin creates a campaign with an empty key, a 7-character key, and a key containing a space
- **THEN** each call returns INVALID_ARGUMENT naming `idempotency_key` and nothing is created
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateCampaignIdempotencyKeyRequired`

#### Scenario: Key is scoped per admin and expires
- **WHEN** admin B uses a key already used by admin A, and the clock then advances 24 h plus one second past A's record
- **THEN** B's call creates its own campaign (another admin's key never blocks it), and a reuse by A with a different payload after expiry creates a new campaign
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignIdempotencyScopeAndExpiry`

#### Scenario: Concurrent duplicates create one campaign
- **WHEN** two calls with the same admin, key and payload run concurrently
- **THEN** exactly one campaign row and one `flash_sale.admin_create` audit entry exist and both calls return its id
- **VERIFIED BY**: team-promotion `internal/store` `TestCampaignIdempotencyConcurrent`

#### Scenario: Seller create needs no key
- **WHEN** a seller creates a campaign for their own listing without an idempotency key
- **THEN** the call returns OK
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateCampaignSellerNoKeyRequired`

#### Scenario: Idempotency lookup precedes validation
- **WHEN** an admin replays a stored (admin, key) pair after the campaign's `ends_at` has passed
- **THEN** the call returns the stored campaign with status ENDED, not INVALID_ARGUMENT, and no listing lookup is made
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignReplayAfterEndsAt`

#### Scenario: Replay after cancel returns current state
- **WHEN** an admin cancels a campaign and then replays its original create request with the same key and payload
- **THEN** the call returns the same campaign id with status CANCELLED and creates nothing
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Replaying create after cancel returns the cancelled campaign"

#### Scenario: Seller sending a key
- **WHEN** a seller creates a campaign with an idempotency_key for a listing owned by someone else, and then for their own listing
- **THEN** the first call returns PERMISSION_DENIED, the second creates a campaign, and no admin idempotency record exists for either call
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignSellerKeyIgnoredOwnershipApplies`

### Requirement: Admin campaign screens

The admin app `/admin/campaigns` SHALL list every campaign with a status filter (all, scheduled, active, ended, cancelled), paging, a cancel action with a confirmation, and an edit action enabled only for scheduled campaigns. The `/admin/campaigns/new` form SHALL generate one idempotency key per form session and reuse it on every retry of the same submit, SHALL validate price, stock cap and window client-side matching the server rules, and SHALL map ALREADY_EXISTS, INVALID_ARGUMENT, `LISTING_UNAVAILABLE`, `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE` to distinct messages, reusing the shared mapping from promotion-admin-vouchers. A `flash_sale_disabled` response SHALL show a banner "flash sale is switched off" instead of an empty-list message. The screens SHALL be reachable only by admin sessions.

#### Scenario: Filter and cancel from the list
- **WHEN** an admin opens `/admin/campaigns`, picks the filter "active" and confirms cancel on one row
- **THEN** the row shows status cancelled and the active filter no longer lists it
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "Admin cancels a campaign from the list screen"

#### Scenario: Edit enabled only for scheduled
- **WHEN** an admin views a scheduled row and an ended row
- **THEN** the edit action is enabled on the scheduled row and absent or disabled on the ended row
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/CampaignRow.test.tsx` "edit only when scheduled"

#### Scenario: Create retry reuses the key
- **WHEN** the create request times out and the admin presses submit again on the same form
- **THEN** the second request carries the same idempotency_key and the list shows one new campaign
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_campaigns.feature` "A create retry after a timeout makes one campaign"

#### Scenario: Server errors map to distinct messages
- **WHEN** create returns ALREADY_EXISTS, INVALID_ARGUMENT for `sale_price`, and UNAVAILABLE with each of `LISTING_UNAVAILABLE`, `AUDIT_UNAVAILABLE` and `PROMOTION_UNAVAILABLE`
- **THEN** five different messages are shown and none contains a raw backend message
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/campaignMessages.test.ts` "maps each error"

#### Scenario: Kill-switch banner
- **WHEN** the list response has flash_sale_disabled true
- **THEN** the banner "flash sale is switched off" is visible and the empty-list message is not
- **VERIFIED BY**: team-frontend-admin `src/features/campaigns/CampaignListView.test.tsx` "shows kill-switch banner"

#### Scenario: Non-admin reaches no campaign screen
- **WHEN** a seller or an anonymous visitor opens `/admin/campaigns`
- **THEN** the visitor is redirected to the admin login or shown forbidden and no campaign data is requested
- **VERIFIED BY**: team-frontend-admin `src/app/admin/campaigns/page.test.tsx` "forbids non-admin"
