@promo @fintech @ads
Feature: Erasing a seller ends their paid plan and their ad campaigns
  A UserErased event on identity.events ends the seller's subscription without a refund and ends every
  campaign of the seller, so the renewal sweep never charges the wallet again and no slot is served.
  (change promotion-seller-erasure-monetization)

  Scenario: An erased seller's plan is never renewed
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller subscribes to "plan_pro" through the gateway
    When a UserErased event for the plan seller is consumed
    And the old period end of the seller's subscription is brought forward and a sweep interval passes
    Then the seller has no subscription
    And the seller's entitlements return FREE
    And the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000

  Scenario: An erased seller's ads stop serving
    Given an ads seller with a published listing in a fresh category
    And the seller has an ACTIVE ad campaign with bid 800 and budget 5000 on that listing
    And a buyer is served a sponsored slot for that listing in that category
    When a UserErased event for the ads seller is consumed
    And a buyer lists the sponsored slots for placement "search" in that category with limit 4
    Then the slots do not contain that listing
    When the buyer records a click for the served token
    Then the click is not charged and names the listing
    And the seller's listed campaign reads status "AD_CAMPAIGN_STATUS_ENDED" and spent 0
