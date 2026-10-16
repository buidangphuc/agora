@promo
Feature: The order service holds flash-sale stock without overselling
  ReserveFlashSaleStock, CommitFlashSaleStock and ReleaseFlashSaleStock are service-only
  RPCs of team-promotion (change promotion-flash-stock-hold). A hold takes units out of the
  live stock meter (GetFlashSaleStock), commit keeps them sold, release returns them (also
  after commit), a hold expires 15 minutes after reserve, and a user principal is refused.
  Campaigns are seeded through CreateCampaign, one listing per scenario so no two cases share stock.

  Scenario: Reserve holds the units of an active campaign
    Given a flash campaign with stock cap 5 and sale price 499000
    When the order service reserves 2 units at the sale price
    Then the reserve is accepted with an empty reason
    And the stock meter shows 3 remaining of 5

  Scenario: Reserve is idempotent on the reservation id
    Given a flash campaign with stock cap 5 and sale price 499000
    When the order service reserves 2 units at the sale price
    And the order service repeats the same reserve
    Then both reserves are accepted
    And the stock meter shows 3 remaining of 5
    And exactly one hold of 2 units exists for the reservation

  Scenario: Reserve refuses an ended campaign
    Given an ended flash campaign with stock cap 5 and sale price 499000
    When the order service reserves 1 units at the sale price
    Then the reserve is refused with reason "not_active"
    And the stock meter shows 5 remaining of 5
    And no hold exists for the reservation

  Scenario: Reserve refuses a price that differs from the campaign
    Given a flash campaign with stock cap 5 and sale price 499000
    When the order service reserves 1 units at a price 1 below the sale price
    Then the reserve is refused with reason "price_changed"
    And the stock meter shows 5 remaining of 5
    And no hold exists for the reservation

  Scenario: Reserve refuses more units than remain
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 4 units at the sale price
    When the order service reserves 2 units at the sale price
    Then the reserve is refused with reason "insufficient_stock"
    And the stock meter shows 1 remaining of 5

  Scenario: Concurrent reserves are capped
    Given a flash campaign with stock cap 3 and sale price 499000
    When 8 single-unit reserves with distinct reservation ids run in parallel
    Then exactly 3 of the parallel reserves are accepted
    And the other 5 parallel reserves are refused with reason "insufficient_stock"
    And the stock meter shows 0 remaining of 3

  Scenario: Commit adds the quantity to stock_sold
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    When the order service commits the reservation
    Then the commit is accepted
    And the stock meter shows 3 remaining of 5
    When the order service commits the reservation
    Then the commit is accepted
    And the stock meter shows 3 remaining of 5

  Scenario: Release returns the units of an active hold
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    When the order service releases the reservation
    Then the release is accepted
    And the stock meter shows 5 remaining of 5

  Scenario: Release returns the units of a committed hold
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    And the order service has committed the reservation
    When the order service releases the reservation
    Then the release is accepted
    And the stock meter shows 5 remaining of 5
    When the order service releases the reservation
    Then the release is accepted
    And the stock meter shows 5 remaining of 5

  Scenario: Commit of an expired hold is refused
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    And the hold is 15 minutes old
    When the order service commits the reservation
    Then the commit is refused
    And the stock meter shows 5 remaining of 5

  Scenario: A buyer cannot reserve flash stock
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    And a registered flash-sale buyer
    When the buyer posts reserve, commit and release for the reservation to the gateway
    Then none of the buyer's gateway calls is served
    When a user principal calls reserve, commit and release directly on team-promotion
    Then every direct call is refused with "permission_denied"
    And the stock meter shows 3 remaining of 5

  Scenario: A user carrying the service scope is still refused
    Given a flash campaign with stock cap 5 and sale price 499000
    And the order service has reserved 2 units at the sale price
    When a user principal carrying the promotion.reserve and admin scopes calls reserve directly
    Then every direct call is refused with "permission_denied"
    And the stock meter shows 3 remaining of 5

  Scenario: Remaining reflects holds and sold units
    Given a flash campaign with stock cap 10 and sale price 499000
    And the order service holds 3 units under reservation "sold"
    And the order service commits reservation "sold"
    And the order service holds 2 units under reservation "active"
    And the order service holds 4 units under reservation "returned"
    And the order service releases reservation "returned"
    Then the stock meter shows 5 remaining of 10

  Scenario: The order service principal is served
    Given a flash campaign with stock cap 5 and sale price 499000
    When the service principal service-team-order with scope promotion.reserve reserves, commits and releases in turn
    Then every direct call returns ok
