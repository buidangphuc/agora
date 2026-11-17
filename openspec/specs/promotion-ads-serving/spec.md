# promotion-ads-serving Specification

## Purpose
Defines how team-promotion serves sponsored slots by placement, category and bid, issues single-use impression tokens, charges a campaign's bid once per click against its budget, and how the gateway exposes the anonymous slot and click calls.

## Requirements

### Requirement: Ad campaign creation enforces the minimum bid and records the category

`CreateAdCampaign` SHALL reject a bid below `MIN_AD_BID` (default 500 VND) and a budget below the bid with `INVALID_ARGUMENT`, and SHALL store on the campaign the `category_id` of the listing as returned by the ownership lookup. A campaign SHALL start ACTIVE with `spent`, `impressions` and `clicks` equal to 0. The seller who owns the listing SHALL be the only non-admin caller able to create it.

#### Scenario: Bid at the minimum is accepted
- **WHEN** a seller creates a campaign for their own listing in category C with bid 500 and budget 5000
- **THEN** the campaign is ACTIVE, has category_id C, spent 0, impressions 0 and clicks 0
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignMinBidAccepted`

#### Scenario: Bid below the minimum is rejected
- **WHEN** a seller creates a campaign with bid 499
- **THEN** the call fails with INVALID_ARGUMENT and no campaign row is written
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignBelowMinBidRejected`

#### Scenario: Budget below the bid is rejected
- **WHEN** a seller creates a campaign with bid 1000 and budget 999
- **THEN** the call fails with INVALID_ARGUMENT and no campaign row is written
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignBudgetBelowBidRejected`

#### Scenario: Another seller cannot advertise the listing
- **WHEN** a seller creates a campaign for a listing owned by another seller
- **THEN** the call fails with PERMISSION_DENIED and no campaign row is written
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignForeignListingDenied`

### Requirement: Sponsored slots are served by placement, category and bid

`ListSponsoredSlots` SHALL accept `placement` `search` or `home`, an optional `category_id` and a `limit` of 1 to 4 (0 means 2). It SHALL consider only campaigns that are ACTIVE with `budget - spent >= bid`, SHALL rank them by bid descending then by oldest campaign first, SHALL return at most one slot per listing (the best campaign of that listing) and at most `limit` slots. For `search` only campaigns whose stored category equals `category_id` SHALL qualify and an empty `category_id` SHALL return no slots; for `home` every category qualifies. An unknown placement or a `limit` outside 0..4 SHALL fail with `INVALID_ARGUMENT`. The response SHALL NOT expose seller id, bid or budget.

#### Scenario: Search serves the best two campaigns of the category
- **WHEN** an anonymous caller lists placement search, category C, limit 2 and C has three ACTIVE campaigns with bids 900, 700 and 500
- **THEN** the response has two slots, for the bid 900 and bid 700 campaigns in that order
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsSearchRanksByBidInCategory`

#### Scenario: Equal bids favour the oldest campaign
- **WHEN** two ACTIVE campaigns in category C have bid 800 and the first was created earlier
- **THEN** the older campaign's slot is returned first
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsTieBreaksOldestFirst`

#### Scenario: Other categories are excluded from search
- **WHEN** an anonymous caller lists placement search for category C and the only higher-bid campaign is in category D
- **THEN** no slot for the category D listing is returned
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsSearchExcludesOtherCategory`

#### Scenario: Search without a category returns no slots
- **WHEN** an anonymous caller lists placement search with an empty category_id while ACTIVE campaigns exist
- **THEN** the response has zero slots
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsSearchRequiresCategory`

#### Scenario: Home serves across categories
- **WHEN** an anonymous caller lists placement home, limit 2, with ACTIVE campaigns in three categories
- **THEN** the two highest-bid campaigns are returned regardless of category
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsHomeIgnoresCategory`

#### Scenario: A listing with two campaigns appears once
- **WHEN** one listing has ACTIVE campaigns with bids 900 and 600 and other listings qualify
- **THEN** the listing appears once, on its bid 900 campaign
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsDedupesListing`

#### Scenario: Paused, budget-exhausted and ended campaigns are not served
- **WHEN** campaigns exist that are PAUSED, PAUSED_BUDGET, ENDED, or ACTIVE with budget - spent below bid
- **THEN** none of them appears in the response
- **VERIFIED BY**: team-promotion `internal/store` `TestListServableExcludesNonServable`

#### Scenario: Limit boundary
- **WHEN** a caller passes limit 4 with six eligible campaigns, then limit 5
- **THEN** the first call returns four slots and the second fails with INVALID_ARGUMENT
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsLimitBounds`

#### Scenario: Unknown placement is rejected
- **WHEN** a caller passes placement "sidebar"
- **THEN** the call fails with INVALID_ARGUMENT
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsUnknownPlacementRejected`

#### Scenario: Slots expose no seller or money data
- **WHEN** an anonymous caller lists placement home
- **THEN** each slot carries only listing_id, campaign_id and impression_token, and the call needs no principal
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListSponsoredSlotsPublicAndMinimal`

#### Scenario: Legacy caller without placement keeps the old answer
- **WHEN** a caller sends only context_str and no placement
- **THEN** listing_ids holds up to 50 distinct listing ids by bid descending, slots is empty and no impression is recorded
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListSlotsLegacyContextOnly`

#### Scenario: Served slots are visible end to end
- **WHEN** a buyer searches a category in which a seller has an ACTIVE campaign (e2e, real stack)
- **THEN** the gateway answers ListSponsoredSlots with that listing in a slot carrying a non-empty impression_token
- **VERIFIED BY**: platform-e2e `ads/sponsored_serving.feature::Served slots are visible end to end`

### Requirement: Every served slot carries a single-use impression token

For each slot returned, `ListSponsoredSlots` SHALL persist one `ad_impressions` row (random opaque token, campaign id, listing id, `expires_at` = now + 30 minutes, no caller identity, IP or session) and SHALL increment the campaign's `impressions` by 1. Tokens SHALL be unguessable (at least 128 bits). A response with zero slots SHALL write nothing.

#### Scenario: Serving records one impression per slot
- **WHEN** a call returns two slots
- **THEN** two distinct tokens exist in ad_impressions expiring 30 minutes after creation and each campaign's impressions rose by 1
- **VERIFIED BY**: team-promotion `internal/store` `TestServeInsertsTokensAndCountsImpressions`

#### Scenario: Empty result writes nothing
- **WHEN** a call finds no eligible campaign
- **THEN** zero ad_impressions rows are inserted and no counter changes
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestServeEmptyWritesNothing`

#### Scenario: A failed token insert fails the call without counting
- **WHEN** the batched token insert fails with a database error
- **THEN** the call returns UNAVAILABLE, no slot is returned and no impressions counter changed
- **VERIFIED BY**: team-promotion `internal/store` `TestServeRollsBackOnInsertFailure`

#### Scenario: The stored row holds no buyer data
- **WHEN** the ad_impressions schema is inspected after the migration
- **THEN** its columns are exactly token, campaign_id, listing_id, created_at, expires_at and clicked_at
- **VERIFIED BY**: team-promotion `internal/store` `TestAdImpressionsSchemaHoldsNoBuyerData`

### Requirement: A click charges the campaign's bid once and burns the budget

`RecordAdClick` SHALL be callable without a principal. For a valid, unexpired, unconsumed token it SHALL, in one transaction, mark the token consumed, add the campaign's bid to `spent`, add 1 to `clicks`, and return `listing_id` with `charged = true`. The budget check and update SHALL be a single compare-and-set so `spent` never exceeds `budget` under concurrent clicks. IF remaining budget after the charge is below the bid, THEN the campaign SHALL become `PAUSED_BUDGET` in the same transaction. A token that is unknown SHALL fail with `NOT_FOUND`.

#### Scenario: First click charges the bid
- **WHEN** an anonymous caller records a click for a fresh token of a campaign with bid 800, budget 5000, spent 0
- **THEN** the response has charged true and the listing id, and the campaign reads spent 800 and clicks 1
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickChargesBidOnce`

#### Scenario: Replaying the token charges nothing
- **WHEN** the same token is clicked a second time
- **THEN** the response has charged false and the listing id, and spent and clicks are unchanged
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickReplayChargesNothing`

#### Scenario: Expired token charges nothing
- **WHEN** a token is clicked 31 minutes after it was issued
- **THEN** the response has charged false and the listing id, and spent is unchanged
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickExpiredChargesNothing`

#### Scenario: Unknown token is not found
- **WHEN** a caller records a click with a token that was never issued
- **THEN** the call fails with NOT_FOUND and no row changes
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickUnknownTokenNotFound`

#### Scenario: Empty token is invalid
- **WHEN** a caller records a click with an empty impression_token
- **THEN** the call fails with INVALID_ARGUMENT
- **VERIFIED BY**: team-promotion `internal/rpc` `TestRecordAdClickEmptyTokenInvalid`

#### Scenario: A seller clicking their own ad is not charged
- **WHEN** the principal whose id equals the campaign's seller records a click with a fresh token
- **THEN** the response has charged false, spent is unchanged and the token is still unconsumed
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickOwnSellerNotCharged`

#### Scenario: Another seller or a buyer clicking is charged
- **WHEN** a principal who is not the campaign's seller records a click with a fresh token
- **THEN** the response has charged true
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickOtherPrincipalCharged`

#### Scenario: The last affordable click pauses the campaign
- **WHEN** a campaign with bid 800, budget 1000 and spent 0 takes its first click
- **THEN** the click is charged, spent is 800 and status is PAUSED_BUDGET
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickExhaustsBudgetPauses`

#### Scenario: A click on a campaign that can no longer pay charges nothing
- **WHEN** a token is clicked after its campaign became PAUSED_BUDGET or paused
- **THEN** the response has charged false and the listing id, and spent is unchanged
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestRecordClickPausedCampaignNotCharged`

#### Scenario: Concurrent clicks never overspend
- **WHEN** 20 concurrent clicks on 20 distinct tokens hit a campaign with bid 500 and budget 2000 against a real database
- **THEN** exactly 4 report charged true, spent equals 2000 and the campaign is PAUSED_BUDGET
- **VERIFIED BY**: team-promotion `internal/store` `TestConcurrentClicksNeverOverspend`

#### Scenario: Concurrent clicks on one token charge once
- **WHEN** 10 concurrent clicks use the same token against a real database
- **THEN** exactly one reports charged true and spent equals one bid
- **VERIFIED BY**: team-promotion `internal/store` `TestConcurrentClicksSameTokenChargeOnce`

#### Scenario: A failed charge rolls the token back
- **WHEN** the campaign update fails with a database error after the token was marked consumed
- **THEN** the call returns UNAVAILABLE and the token is still unconsumed
- **VERIFIED BY**: team-promotion `internal/store` `TestRecordClickRollsBackOnChargeFailure`

#### Scenario: Click and budget burn end to end
- **WHEN** a buyer clicks a served sponsored slot twice through the gateway (e2e, real stack)
- **THEN** the first RecordAdClick is charged, the second is not, and the seller's campaign shows spent equal to one bid
- **VERIFIED BY**: platform-e2e `ads/sponsored_click.feature::Click and budget burn end to end`

#### Scenario: Budget exhaustion stops serving end to end
- **WHEN** clicks burn a campaign's budget below its bid (e2e, real stack)
- **THEN** the next ListSponsoredSlots for its category no longer contains the listing
- **VERIFIED BY**: platform-e2e `ads/sponsored_click.feature::Budget exhaustion stops serving end to end`

### Requirement: The gateway exposes the anonymous click and slot calls without ad logic

team-gateway SHALL forward `SponsoredService.RecordAdClick` and `SponsoredService.ListSponsoredSlots` for callers without credentials and with a valid buyer, forwarding the resolved principal and applying no ad logic or scope check. A request with a malformed bearer token SHALL be answered `UNAUTHENTICATED` and not forwarded as anonymous.

#### Scenario: Anonymous caller reaches RecordAdClick
- **WHEN** a request without credentials calls RecordAdClick through the gateway with a valid token
- **THEN** the gateway forwards it and the answer is not 401 or 403
- **VERIFIED BY**: team-gateway `internal/edge` `TestRecordAdClickForwardsAnonymous`

#### Scenario: A logged-in buyer's principal is forwarded
- **WHEN** a logged-in buyer calls RecordAdClick through the gateway
- **THEN** the upstream request carries that buyer's principal
- **VERIFIED BY**: team-gateway `internal/edge` `TestRecordAdClickForwardsPrincipal`

#### Scenario: Malformed bearer token is rejected
- **WHEN** a request with a malformed bearer token calls RecordAdClick through the gateway
- **THEN** the gateway answers HTTP 401 and does not call team-promotion
- **VERIFIED BY**: team-gateway `internal/edge` `TestRecordAdClickRejectsBadBearer`

### Requirement: Old impression rows are purged

The team-promotion sweeper SHALL delete `ad_impressions` rows whose `created_at` is older than 30 days, in batches of at most 100 per statement, and SHALL leave newer rows and all `ad_campaigns` counters untouched.

#### Scenario: Rows older than 30 days are deleted
- **WHEN** the sweeper runs with rows aged 31 days and 29 days
- **THEN** the 31-day row is deleted and the 29-day row remains
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweeperPurgesOldImpressions`

#### Scenario: Purge failure does not stop the voucher sweep
- **WHEN** the impression delete fails with a database error during a sweep
- **THEN** the sweep logs the error, the timed-out voucher issuance still completes and the next tick retries
- **VERIFIED BY**: team-promotion `internal/consumer` `TestSweeperPurgeFailureDoesNotBlockVouchers`

### Requirement: The migration backfills existing campaigns

The migration SHALL add `spent`, `impressions`, `clicks` (0) and `category_id` (empty) to existing `ad_campaigns` rows and SHALL set to ENDED every existing campaign whose bid is below `MIN_AD_BID` or whose budget is below its bid. It SHALL NOT change other rows' status.

#### Scenario: Legacy campaigns that could never serve are ended
- **WHEN** the migration runs over rows with bid 0, bid 499, budget below bid, and a valid bid 800 budget 5000 ACTIVE row
- **THEN** the first three are ENDED and the valid row stays ACTIVE with spent 0
- **VERIFIED BY**: team-promotion `internal/store` `TestMigrationEndsUnservableLegacyCampaigns`

#### Scenario: The migration reverses cleanly
- **WHEN** the down migration runs after the up migration
- **THEN** the new columns and the ad_impressions table are gone and the pre-existing columns are intact
- **VERIFIED BY**: team-promotion `internal/store` `TestMigrationDownRestoresSchema`
