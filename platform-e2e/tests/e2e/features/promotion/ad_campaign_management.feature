@promo @ads
Feature: Sellers manage their ad campaigns through the gateway
  A seller lists, pauses and resumes their own sponsored campaigns; resuming needs the remaining
  budget to cover the bid and no seller can touch another seller's campaign.

  Scenario: Seller sees campaigns after reload through the gateway
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    When the seller lists their campaigns in a new request
    Then the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ACTIVE" and spent 0

  Scenario: Seller pauses a campaign through the gateway
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    When the seller pauses the campaign
    Then the campaign call succeeds with status "AD_CAMPAIGN_STATUS_PAUSED"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_PAUSED" and spent 0
    When a buyer lists the sponsored slots for placement "search" in that category with limit 4
    Then the slots do not contain that listing

  Scenario: Seller resumes a paused campaign through the gateway
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    When the seller pauses the campaign
    And the seller resumes the campaign
    Then the campaign call succeeds with status "AD_CAMPAIGN_STATUS_ACTIVE"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ACTIVE" and spent 0
    When a buyer lists the sponsored slots for placement "search" in that category with limit 4
    Then the slots contain that listing with a non-empty impression token

  Scenario: Resume without budget is refused through the gateway
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 1000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When the buyer records a click for the served token
    And the seller pauses the campaign
    And the seller resumes the campaign
    Then the campaign call fails with code "failed_precondition"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_PAUSED" and spent 800

  Scenario: Another seller cannot manage a campaign through the gateway
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    And a second seller
    When the second seller pauses the first seller's campaign
    Then the campaign call fails with code "not_found"
    When the second seller resumes the first seller's campaign
    Then the campaign call fails with code "not_found"
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ACTIVE" and spent 0
