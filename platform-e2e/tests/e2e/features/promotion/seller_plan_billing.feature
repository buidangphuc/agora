@promo @fintech
Feature: A seller pays for a plan from the wallet and the plan follows the paid term
  Subscribe charges the seller wallet through team-payment, CancelSubscription stops renewal
  and the sweep lapses a cancelled plan, all through the public gateway. (promotion-plan-billing)

  Scenario: Seller upgrades to PRO and the wallet is debited
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    When the seller subscribes to "plan_pro" through the gateway
    Then the subscription call succeeds
    And the seller's subscription is PRO ACTIVE with auto-renew and a period end about 30 days ahead
    And the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000

  Scenario: Seller without funds cannot upgrade
    Given a seller with no wallet funds
    When the seller subscribes to "plan_pro" through the gateway
    Then the subscription call fails with FAILED_PRECONDITION
    And the seller has no subscription
    And the seller's ledger has no SUBSCRIPTION_FEE entry

  Scenario: Cancelled plan lapses at period end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller subscribes to "plan_pro" through the gateway
    When the seller cancels the subscription through the gateway
    And the period end of the seller's subscription is brought forward and the sweep runs
    Then the seller's entitlements return FREE
    And the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000

  Scenario: Another seller cannot cancel my plan
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller subscribes to "plan_pro" through the gateway
    And another seller "mallory"
    When "mallory" cancels the subscription through the gateway
    Then "mallory" has no subscription
    And the first seller's subscription still has auto-renew
