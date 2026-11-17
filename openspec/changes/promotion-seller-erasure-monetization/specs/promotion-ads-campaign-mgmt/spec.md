## ADDED Requirements

### Requirement: Erasing a seller ends their ad campaigns

On `identity.events/UserErased` team-promotion SHALL, in the same transaction as the voucher erasure and keyed only by the event's `user_id`, set every `ad_campaigns` row whose `seller_id` is the user and whose status is ACTIVE, PAUSED or PAUSED_BUDGET to ENDED, keeping `budget`, `spent`, `impressions`, `clicks`, `listing_id` and `seller_id`. Rows already ENDED and other sellers' rows SHALL be unchanged. An ended campaign SHALL NOT be served, SHALL NOT be charged for a click on an impression token issued before the erasure, and SHALL NOT be resumable. No budget SHALL be refunded. Redelivery SHALL change nothing.

#### Scenario: All non-ended campaigns of the seller end
- **GIVEN** seller U has one ACTIVE, one PAUSED, one PAUSED_BUDGET and one ENDED campaign, the ACTIVE one with spent 800 and clicks 1, and seller W has one ACTIVE campaign
- **WHEN** a `UserErased` event for U is consumed
- **THEN** all four of U's campaigns are ENDED with their budget, spent, impressions and clicks unchanged, and W's campaign is unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserEndsAdCampaigns`

#### Scenario: An ended campaign is neither served nor charged
- **GIVEN** U's ACTIVE campaign was served and an impression token issued, and then U was erased
- **WHEN** the slots for that category are listed and the token is clicked
- **THEN** the slots do not contain U's campaign, the click is not charged and the campaign's spent and clicks are unchanged
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserCampaignsNotServed`

#### Scenario: Redelivery leaves the ended campaigns unchanged
- **GIVEN** a `UserErased` event for U was consumed
- **WHEN** the same event and a second event for U are consumed
- **THEN** both complete without error and U's campaign rows are unchanged by them
- **VERIFIED BY**: team-promotion `internal/store` `TestEraseUserIdempotent`

#### Scenario: An erased seller's ads stop serving on the real stack
- **GIVEN** an ads seller with an ACTIVE campaign that a buyer is served in a fresh category
- **WHEN** a `UserErased` event for that seller is consumed
- **THEN** the slots for that category no longer contain the listing, a click on the earlier token is not charged, and the seller's listed campaign reads status ENDED and spent 0
- **VERIFIED BY**: platform-e2e `features/promotion/seller_erasure_monetization.feature` "An erased seller's ads stop serving"
