@seller @promo @fintech
Feature: The seller pays for, cancels and reads a plan on the plans page
  The /seller/plans page shows each plan's price per term and the current plan with its period,
  confirms a subscription before charging the wallet and lets the seller stop auto-renew.
  Money is the mock payment only. (seller-plan-ui)

  Scenario: Seller upgrades to PRO from the wallet balance
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the plan-ui browser is logged in as the seller
    When the plan-ui seller opens the plans page
    And the plan-ui seller upgrades to PRO and confirms
    And the plan-ui seller reloads the plans page
    Then the plan-ui PRO card is the current plan with a period end about 30 days ahead
    And the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000

  Scenario: Subscribing with too little balance changes nothing
    Given a seller with a listing "L" whose order pays 100000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the plan-ui browser is logged in as the seller
    When the plan-ui seller opens the plans page
    And the plan-ui seller upgrades to PRO and confirms
    Then the plan-ui insufficient balance message links to the wallet and an error toast shows
    When the plan-ui seller reloads the plans page
    Then the plan-ui FREE card is the current plan
    And the seller has no subscription
    And the seller's ledger has no SUBSCRIPTION_FEE entry

  Scenario: Seller cancels renewal and keeps PRO until period end
    Given a seller with a listing "L" whose order pays 500000
    And a buyer "b1"
    And "b1" has paid an order of "L" whose credit has aged past the hold window
    And the seller subscribes to "plan_pro" through the gateway
    And the plan-ui browser is logged in as the seller
    When the plan-ui seller opens the plans page
    And the plan-ui seller cancels renewal and confirms
    And the plan-ui seller reloads the plans page
    Then the plan-ui PRO card is the current plan with renewal off and a lapse date
    And the plan-ui wallet has no refund entry
    And the seller's ledger holds exactly one SUBSCRIPTION_FEE of -199000
