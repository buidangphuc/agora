# promotion-ads-campaign-mgmt Specification

## ADDED Requirements

### Requirement: A seller lists only their own ad campaigns

`SponsoredService.ListMyAdCampaigns` SHALL return the calling seller's campaigns, newest first (`created_at DESC, id DESC`), each with listing_id, bid, budget, spent, impressions, clicks and status as stored, paged by `PageRequest` (default page_size 50, clamped to 100). The seller id SHALL come from the authenticated principal; the request carries none. The caller SHALL hold the `listing.write` scope.

#### Scenario: Campaigns survive a reload with their counters
- **WHEN** seller S created two campaigns and a buyer click has charged one of them 800 VND, then S calls ListMyAdCampaigns in a fresh session
- **THEN** the response lists both campaigns newest first and the charged one reports spent 800, clicks 1 and the stored impressions
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyAdCampaignsReturnsOwnNewestFirstWithCounters`

#### Scenario: Another seller's campaigns are not listed
- **WHEN** seller S has no campaign and seller T has three, and S calls ListMyAdCampaigns
- **THEN** the response contains zero campaigns and S sees none of T's
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyAdCampaignsExcludesOtherSellers`

#### Scenario: Pagination walks all campaigns without repeats
- **WHEN** seller S owns 120 campaigns and S pages with page_size 100 then follows next_cursor
- **THEN** the first page holds 100 campaigns, the second holds 20, next_cursor is empty after the second and no id appears twice
- **VERIFIED BY**: team-promotion `internal/store` `TestListBySellerCursorPaging`

#### Scenario: Oversized page size is clamped
- **WHEN** S calls ListMyAdCampaigns with page_size 100000
- **THEN** at most 100 campaigns are returned and the call succeeds
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestListMyAdCampaignsClampsPageSize`

#### Scenario: Seller with no campaigns gets an empty list
- **WHEN** a seller with no campaign calls ListMyAdCampaigns
- **THEN** the response has zero campaigns and an empty next_cursor
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyAdCampaignsEmpty`

#### Scenario: Caller without the seller scope is denied
- **WHEN** a buyer principal without `listing.write` calls ListMyAdCampaigns
- **THEN** the call fails with PERMISSION_DENIED and no store query runs
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyAdCampaignsRequiresSellerScope`

#### Scenario: Unauthenticated caller is rejected
- **WHEN** a request without a principal calls ListMyAdCampaigns
- **THEN** the call fails with UNAUTHENTICATED
- **VERIFIED BY**: team-promotion `internal/rpc` `TestListMyAdCampaignsUnauthenticated`

#### Scenario: Seller sees campaigns after reload through the gateway
- **WHEN** seller S creates a campaign through the gateway, then a new session of S calls ListMyAdCampaigns through the gateway
- **THEN** the response contains that campaign with status ACTIVE and spent 0
- **VERIFIED BY**: platform-e2e `promotion/ad_campaign_management.feature::Seller sees campaigns after reload through the gateway`

### Requirement: A seller pauses their own campaign

`SponsoredService.PauseAdCampaign` SHALL move an owned campaign to PAUSED with one conditional UPDATE scoped by `id AND seller_id` and return the updated campaign. ACTIVE and PAUSED_BUDGET campaigns become PAUSED, a PAUSED campaign stays PAUSED (idempotent success), and an ENDED campaign SHALL be rejected with FAILED_PRECONDITION. A campaign that is missing or owned by another seller SHALL return NOT_FOUND. A paused campaign SHALL NOT be returned by ListSponsoredSlots.

#### Scenario: Active campaign is paused and stops being served
- **WHEN** seller S pauses their ACTIVE campaign C
- **THEN** the response reports status PAUSED and a following ListSponsoredSlots for C's category does not return C
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPauseAdCampaignStopsServing`

#### Scenario: Pausing twice succeeds and changes nothing
- **WHEN** S pauses the already PAUSED campaign C again
- **THEN** the call succeeds with status PAUSED and spent, clicks and budget are unchanged
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestPauseAdCampaignIdempotent`

#### Scenario: Budget-paused campaign can be paused by the seller
- **WHEN** S pauses campaign C whose status is PAUSED_BUDGET
- **THEN** the response reports status PAUSED
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestPauseAdCampaignFromBudgetPaused`

#### Scenario: Ended campaign cannot be paused
- **WHEN** S pauses campaign C whose status is ENDED
- **THEN** the call fails with FAILED_PRECONDITION and the status stays ENDED
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestPauseAdCampaignEndedRejected`

#### Scenario: Another seller's campaign is NOT_FOUND on pause
- **WHEN** seller T pauses campaign C owned by seller S
- **THEN** the call fails with NOT_FOUND and C stays ACTIVE
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPauseAdCampaignOtherSellerNotFound`

#### Scenario: Unknown campaign id is NOT_FOUND on pause
- **WHEN** S pauses a campaign id that does not exist
- **THEN** the call fails with NOT_FOUND, the same code and message as for another seller's campaign
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPauseAdCampaignUnknownIDNotFound`

#### Scenario: Empty campaign id is rejected
- **WHEN** S calls PauseAdCampaign with an empty campaign_id
- **THEN** the call fails with INVALID_ARGUMENT
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPauseAdCampaignEmptyIDInvalid`

#### Scenario: Caller without the seller scope cannot pause
- **WHEN** a buyer principal without `listing.write` calls PauseAdCampaign
- **THEN** the call fails with PERMISSION_DENIED and no campaign changes
- **VERIFIED BY**: team-promotion `internal/rpc` `TestPauseAdCampaignRequiresSellerScope`

#### Scenario: Seller pauses a campaign through the gateway
- **WHEN** seller S pauses an ACTIVE campaign through the gateway and lists campaigns again
- **THEN** the listed campaign shows status PAUSED
- **VERIFIED BY**: platform-e2e `promotion/ad_campaign_management.feature::Seller pauses a campaign through the gateway`

### Requirement: A seller resumes their own campaign only while budget remains

`SponsoredService.ResumeAdCampaign` SHALL move an owned PAUSED campaign to ACTIVE with one conditional UPDATE that requires `budget - spent >= bid`, and return the updated campaign. An ACTIVE campaign stays ACTIVE (idempotent success). A PAUSED campaign whose remaining budget is below its bid, a PAUSED_BUDGET campaign and an ENDED campaign SHALL be rejected with FAILED_PRECONDITION and left unchanged. A campaign that is missing or owned by another seller SHALL return NOT_FOUND.

#### Scenario: Paused campaign with budget resumes and is served again
- **WHEN** S resumes PAUSED campaign C with budget 10000, spent 1000 and bid 800
- **THEN** the response reports status ACTIVE and ListSponsoredSlots for C's category returns C again
- **VERIFIED BY**: team-promotion `internal/rpc` `TestResumeAdCampaignServesAgain`

#### Scenario: Boundary remaining budget equal to the bid resumes
- **WHEN** S resumes PAUSED campaign C with budget 5000, spent 4200 and bid 800
- **THEN** the response reports status ACTIVE
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestResumeAdCampaignRemainingEqualsBid`

#### Scenario: Remaining budget below the bid cannot resume
- **WHEN** S resumes PAUSED campaign C with budget 5000, spent 4300 and bid 800
- **THEN** the call fails with FAILED_PRECONDITION and C stays PAUSED
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestResumeAdCampaignInsufficientBudgetRejected`

#### Scenario: Budget-exhausted campaign cannot be resumed
- **WHEN** S resumes campaign C whose status is PAUSED_BUDGET
- **THEN** the call fails with FAILED_PRECONDITION and C stays PAUSED_BUDGET
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestResumeAdCampaignPausedBudgetRejected`

#### Scenario: Ended campaign cannot be resumed
- **WHEN** S resumes campaign C whose status is ENDED
- **THEN** the call fails with FAILED_PRECONDITION and C stays ENDED
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestResumeAdCampaignEndedRejected`

#### Scenario: Resuming an active campaign succeeds and changes nothing
- **WHEN** S resumes the already ACTIVE campaign C
- **THEN** the call succeeds with status ACTIVE and spent is unchanged
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestResumeAdCampaignIdempotent`

#### Scenario: Resume racing the last click never overspends
- **WHEN** campaign C is PAUSED with remaining budget exactly equal to bid, and a buyer click and ResumeAdCampaign run concurrently against a real database
- **THEN** the final state is ACTIVE (click ignored as C was not ACTIVE) or PAUSED_BUDGET (resume then click consumed the last bid), never ACTIVE with remaining < bid, and spent <= budget in all 100 repetitions
- **VERIFIED BY**: team-promotion `internal/store` `TestResumeVersusClickNeverOverspends`

#### Scenario: Another seller's campaign is NOT_FOUND on resume
- **WHEN** seller T resumes PAUSED campaign C owned by seller S
- **THEN** the call fails with NOT_FOUND and C stays PAUSED
- **VERIFIED BY**: team-promotion `internal/rpc` `TestResumeAdCampaignOtherSellerNotFound`

#### Scenario: Caller without the seller scope cannot resume
- **WHEN** a buyer principal without `listing.write` calls ResumeAdCampaign
- **THEN** the call fails with PERMISSION_DENIED and no campaign changes
- **VERIFIED BY**: team-promotion `internal/rpc` `TestResumeAdCampaignRequiresSellerScope`

#### Scenario: Seller resumes a paused campaign through the gateway
- **WHEN** seller S pauses then resumes a campaign through the gateway and lists campaigns
- **THEN** the listed campaign shows status ACTIVE
- **VERIFIED BY**: platform-e2e `promotion/ad_campaign_management.feature::Seller resumes a paused campaign through the gateway`

#### Scenario: Resume without budget is refused through the gateway
- **WHEN** seller S resumes a paused campaign whose remaining budget is below its bid through the gateway
- **THEN** the gateway returns a failed-precondition error and the campaign list still shows PAUSED
- **VERIFIED BY**: platform-e2e `promotion/ad_campaign_management.feature::Resume without budget is refused through the gateway`

#### Scenario: Another seller cannot manage a campaign through the gateway
- **WHEN** seller T pauses or resumes seller S's campaign through the gateway
- **THEN** the gateway returns not-found and S's campaign status is unchanged
- **VERIFIED BY**: platform-e2e `promotion/ad_campaign_management.feature::Another seller cannot manage a campaign through the gateway`

### Requirement: The gateway forwards campaign management to team-promotion with the caller identity

team-gateway SHALL expose `ListMyAdCampaigns`, `PauseAdCampaign` and `ResumeAdCampaign` on its SponsoredService edge, forwarding the caller's credentials unchanged and requiring an authenticated session; it SHALL map upstream status codes without rewriting them.

#### Scenario: Anonymous request is rejected at the edge
- **WHEN** a request without a session calls ListMyAdCampaigns on the gateway
- **THEN** the gateway returns unauthenticated and team-promotion receives no call
- **VERIFIED BY**: team-gateway `internal/edge/promotion_ad_campaigns_test.go` `TestPromotionAdCampaigns_AnonymousRejectedAtEdge`

#### Scenario: Upstream NOT_FOUND is passed through
- **WHEN** team-promotion returns NOT_FOUND for PauseAdCampaign
- **THEN** the gateway returns not-found to the caller
- **VERIFIED BY**: team-gateway `internal/edge/promotion_ad_campaigns_test.go` `TestPromotionAdCampaigns_UpstreamNotFoundPassedThrough`

#### Scenario: Upstream unavailable is surfaced
- **WHEN** team-promotion is unavailable while ListMyAdCampaigns is forwarded
- **THEN** the gateway returns unavailable
- **VERIFIED BY**: team-gateway `internal/edge/promotion_ad_campaigns_test.go` `TestPromotionAdCampaigns_UpstreamUnavailableSurfaced`
