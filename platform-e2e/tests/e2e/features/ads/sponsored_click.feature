@promo @ads
Feature: Sponsored clicks charge the campaign budget
  A click on a served slot charges the campaign's bid once per impression token and a campaign
  that can no longer afford its bid stops being served.

  Scenario: Click charges the bid once
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When the buyer records a click for the served token
    Then the click is charged and names the listing
    When the buyer records a click for the served token
    Then the click is not charged and names the listing
    And the seller's campaign reads spent 800 and clicks 1 with status "AD_CAMPAIGN_STATUS_ACTIVE"

  Scenario: Exhausted campaign stops being served
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 1000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When the buyer records a click for the served token
    Then the click is charged and names the listing
    And the seller's campaign reads spent 800 and clicks 1 with status "AD_CAMPAIGN_STATUS_PAUSED_BUDGET"
    When a buyer lists the sponsored slots for placement "search" in that category with limit 4
    Then the slots do not contain that listing
