# listing-plan-cap Specification

## Purpose
Defines how team-domain caps a seller's listing count and storefront featured listings by the seller's plan entitlements from team-promotion, failing closed when the plan cannot be read and never hiding existing listings on downgrade.

## Requirements

### Requirement: CreateListing enforces the seller plan's max_listings

`CreateListing` SHALL read `max_listings` for the authenticated seller from `SubscriptionService.GetEntitlements` and SHALL reject the create with `RESOURCE_EXHAUSTED` and `ErrorInfo.Reason = "PLAN_LISTING_LIMIT"` when the seller's total listing count, across every status, is greater than or equal to the cap. A cap of `-1` SHALL mean unlimited. The count and the insert SHALL run in one transaction serialised per seller, so concurrent creates never exceed the cap.

#### Scenario: FREE seller at the cap is rejected
- **GIVEN** a seller on FREE (max_listings 10) who owns 10 listings of mixed statuses
- **WHEN** the seller calls CreateListing for an 11th
- **THEN** the call returns RESOURCE_EXHAUSTED with reason PLAN_LISTING_LIMIT and the seller still owns exactly 10 listings
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateRejectedAtPlanCap`

#### Scenario: Create just under the cap succeeds
- **GIVEN** a seller on FREE who owns 9 listings
- **WHEN** the seller calls CreateListing
- **THEN** the call succeeds and the seller owns exactly 10 listings
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateAllowedBelowPlanCap`

#### Scenario: Every status counts toward the cap
- **GIVEN** a seller on FREE who owns 4 published, 3 draft, 2 hidden and 1 sold-out listing
- **WHEN** the seller calls CreateListing
- **THEN** the call returns RESOURCE_EXHAUSTED with reason PLAN_LISTING_LIMIT
- **VERIFIED BY**: team-domain `internal/store` `TestCountSellerListingsAllStatuses`

#### Scenario: Unlimited plan is never blocked
- **GIVEN** a seller whose entitlements report max_listings "-1" and who owns 600 listings
- **WHEN** the seller calls CreateListing
- **THEN** the call succeeds
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateUnlimitedPlanNeverBlocked`

#### Scenario: Concurrent creates cannot overshoot the cap
- **GIVEN** a seller on FREE who owns 8 listings against a real database
- **WHEN** 6 CreateListing calls for that seller run concurrently
- **THEN** exactly 2 succeed, 4 return RESOURCE_EXHAUSTED and the seller owns exactly 10 listings
- **VERIFIED BY**: team-domain `internal/store` `TestConcurrentCreatesRespectPlanCap`

#### Scenario: Cap is evaluated for the caller's own seller id
- **GIVEN** seller A on FREE at the cap and seller B on FREE with 0 listings
- **WHEN** seller B calls CreateListing with a body whose seller_id is A's id
- **THEN** the call succeeds, the listing is owned by B and A's count is unchanged
- **VERIFIED BY**: team-domain `internal/rpc` `TestCreateCapUsesPrincipalNotBodySeller`

#### Scenario: Caller without listing.write is refused before any entitlement call
- **WHEN** a principal lacking the `listing.write` scope calls CreateListing
- **THEN** the call returns PERMISSION_DENIED and the entitlement port is called 0 times
- **VERIFIED BY**: team-domain `internal/rpc` `TestCreateWithoutScopeSkipsEntitlementLookup`

### Requirement: Plan lookup failure fails closed

When team-promotion cannot be reached, times out, or returns a response that lacks the required limit key or carries a non-integer value, `CreateListing` and the storefront featured-ids write SHALL return `UNAVAILABLE`, SHALL insert or change nothing, and SHALL NOT treat the failure as unlimited. Reads, updates and deletes of listings SHALL NOT depend on team-promotion.

#### Scenario: team-promotion down blocks create
- **GIVEN** the entitlement upstream returns a connection error
- **WHEN** a seller with 0 listings calls CreateListing
- **THEN** the call returns UNAVAILABLE and zero listings and zero outbox rows are written
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateFailsClosedWhenPromotionUnavailable`

#### Scenario: Upstream timeout blocks create
- **GIVEN** the entitlement upstream does not answer within the 2 s timeout
- **WHEN** a seller calls CreateListing
- **THEN** the call returns UNAVAILABLE within 3 s and nothing is written
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateFailsClosedOnUpstreamTimeout`

#### Scenario: Malformed limits are not unlimited
- **GIVEN** the entitlement response has no `max_listings` key, or the value "abc"
- **WHEN** a seller calls CreateListing
- **THEN** the call returns UNAVAILABLE and nothing is written
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateFailsClosedOnMalformedLimits`

#### Scenario: Update, delete and read work while team-promotion is down
- **GIVEN** the entitlement upstream is unreachable and a seller owns 12 listings
- **WHEN** the seller updates one, deletes one and lists their listings
- **THEN** all three calls succeed and the entitlement port is called 0 times
- **VERIFIED BY**: team-domain `internal/rpc` `TestNonCreatePathsIgnoreEntitlements`

### Requirement: Downgrade never removes or hides existing listings

A seller whose plan cap is lower than their listing count SHALL keep every existing listing unchanged and editable; only new creates SHALL be blocked until the count drops below the cap.

#### Scenario: Over-cap seller keeps listings and can edit them
- **GIVEN** a seller now on FREE who owns 14 listings (cap 10)
- **WHEN** the seller lists, updates one and calls CreateListing
- **THEN** the list returns 14 listings, the update succeeds, and the create returns RESOURCE_EXHAUSTED with reason PLAN_LISTING_LIMIT
- **VERIFIED BY**: team-domain `internal/listing` `TestOverCapSellerKeepsListings`

#### Scenario: Deleting below the cap re-enables create
- **GIVEN** a seller on FREE who owns 11 listings
- **WHEN** the seller deletes 2 listings and then calls CreateListing
- **THEN** the create succeeds and the seller owns 10 listings
- **VERIFIED BY**: team-domain `internal/listing` `TestDeleteBelowCapReenablesCreate`

#### Scenario: Zero cap blocks every create
- **GIVEN** a plan whose max_listings is "0"
- **WHEN** a seller with 0 listings calls CreateListing
- **THEN** the create is rejected with RESOURCE_EXHAUSTED and reason PLAN_LISTING_LIMIT and no listing is created
- **VERIFIED BY**: team-domain `internal/listing` `TestCreateZeroCapBlocks`

#### Scenario: Over-cap status does not let another seller touch the listings
- **GIVEN** seller A is over the cap and seller B is not the owner
- **WHEN** seller B updates one of A's listings
- **THEN** the call is rejected with PERMISSION_DENIED and the listing is not changed
- **VERIFIED BY**: team-domain `internal/rpc` `TestOverCapListingsStillOwnerOnly`

### Requirement: Storefront featured ids are capped by featured_listings

`UpsertStorefront` SHALL reject a write whose `featured_listing_ids` count exceeds the seller's `featured_listings` entitlement with `RESOURCE_EXHAUSTED` and reason `PLAN_FEATURED_LIMIT`; the plan value SHALL replace any fixed limit of 6. A count equal to the cap SHALL be accepted. An already stored storefront SHALL NOT be rewritten when the cap later drops.

#### Scenario: Featured ids above the plan cap are rejected
- **GIVEN** a seller on PRO (featured_listings 5)
- **WHEN** the seller upserts a storefront with 6 featured_listing_ids
- **THEN** the call returns RESOURCE_EXHAUSTED with reason PLAN_FEATURED_LIMIT and the stored storefront is unchanged
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontFeaturedAbovePlanCapRejected`

#### Scenario: Featured ids equal to the cap are accepted
- **GIVEN** a seller on PRO (featured_listings 5)
- **WHEN** the seller upserts a storefront with 5 featured_listing_ids
- **THEN** the call succeeds and the storefront stores 5 ids
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontFeaturedAtPlanCapAccepted`

#### Scenario: Plan cap overrides the fixed limit of 6
- **GIVEN** a seller on PREMIUM (featured_listings 50)
- **WHEN** the seller upserts a storefront with 9 featured_listing_ids
- **THEN** the call succeeds and the storefront stores 9 ids
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontFeaturedPlanCapOverridesFixedLimit`

#### Scenario: FREE seller cannot feature listings
- **GIVEN** a seller on FREE (featured_listings 0)
- **WHEN** the seller upserts a storefront with 1 featured_listing_id
- **THEN** the call returns RESOURCE_EXHAUSTED with reason PLAN_FEATURED_LIMIT, and an upsert with 0 featured ids succeeds
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontFreeSellerCannotFeature`

#### Scenario: Another seller's storefront cannot be written through the cap path
- **GIVEN** seller B on PREMIUM and a storefront body whose seller_id is seller A's id
- **WHEN** seller B upserts it with 3 featured_listing_ids
- **THEN** the stored storefront belongs to B, the cap used is B's plan, and A's storefront is not changed
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontCapUsesPrincipalNotBodySeller`

#### Scenario: Storefront write fails closed
- **GIVEN** the entitlement upstream is unreachable
- **WHEN** a seller upserts a storefront with 1 featured_listing_id
- **THEN** the call returns UNAVAILABLE and the stored storefront is unchanged
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontFeaturedFailsClosed`

#### Scenario: Storefront without featured ids does not need the plan
- **GIVEN** the entitlement upstream is unreachable
- **WHEN** a seller upserts a storefront with 0 featured_listing_ids
- **THEN** the call succeeds
- **VERIFIED BY**: team-domain `internal/listing` `TestStorefrontWithoutFeaturedIgnoresEntitlements`

### Requirement: team-promotion address is required configuration

team-domain SHALL require `UPSTREAM_PROMOTION_ADDR` and SHALL refuse to start with a configuration error naming it when it is empty or blank; there SHALL be no enable flag and no in-memory fallback.

#### Scenario: Missing address fails startup
- **WHEN** team-domain loads configuration with `UPSTREAM_PROMOTION_ADDR` unset or blank
- **THEN** loading returns an error that contains `UPSTREAM_PROMOTION_ADDR`
- **VERIFIED BY**: team-domain `internal/config` `TestPromotionAddrRequired`

#### Scenario: Configured address is declared in settings
- **WHEN** team-domain loads configuration with `UPSTREAM_PROMOTION_ADDR=team-promotion:50061`
- **THEN** the loaded upstream settings carry that address and the settings list declares the key
- **VERIFIED BY**: team-domain `internal/config` `TestPromotionAddrLoaded`

### Requirement: The cap is observable end to end

On the running stack, a seller on each plan SHALL be limited by that plan's `max_listings` when creating through the gateway, and the rejection SHALL carry the stable reason `PLAN_LISTING_LIMIT` that the seller UI keys on.

#### Scenario: FREE seller is blocked at the 11th listing end to end
- **GIVEN** a seeded FREE seller who owns 10 listings on the running stack
- **WHEN** the seller creates an 11th listing through the gateway
- **THEN** the response is RESOURCE_EXHAUSTED with reason PLAN_LISTING_LIMIT and the seller's listing list still has 10 entries
- **VERIFIED BY**: platform-e2e `listing/listing_plan_cap.feature::FREE seller is blocked at the 11th listing end to end`

#### Scenario: PRO seller is not blocked at the 11th listing end to end
- **GIVEN** a seeded PRO seller who owns 10 listings on the running stack
- **WHEN** the seller creates an 11th listing through the gateway
- **THEN** the create succeeds and the seller's listing list has 11 entries
- **VERIFIED BY**: platform-e2e `listing/listing_plan_cap.feature::PRO seller is not blocked at the 11th listing end to end`

#### Scenario: Over-cap seller keeps their listings end to end
- **GIVEN** a seeded seller on FREE who owns 12 listings
- **WHEN** the seller opens their listings
- **THEN** all 12 are returned and none is hidden or removed
- **VERIFIED BY**: platform-e2e `listing/listing_plan_cap.feature::Over-cap seller keeps their listings end to end`

#### Scenario: Another seller's listings are not exposed to the capped seller
- **GIVEN** the running stack with seller A at the cap and seller B owning 3 listings
- **WHEN** seller A lists their listings and seller B's token is not used
- **THEN** only A's listings are returned and none of B's listings is included
- **VERIFIED BY**: platform-e2e `listing/listing_plan_cap.feature::Another seller's listings are not exposed to the capped seller`
