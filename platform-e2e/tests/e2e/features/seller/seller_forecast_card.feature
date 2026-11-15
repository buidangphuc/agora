@seller
Feature: Seller analytics shows a demand forecast card
  The /seller/analytics page renders "Dự báo nhu cầu 14 ngày" for the top listings of the range, with the
  numbers taken from GetDemandForecast through the gateway, never recomputed or invented. A listing
  without a forecast key says there is not enough data, a failing store shows no numbers and leaves
  the page intact, and a seller cannot read another seller's forecast. Magic Listing beacons carry the
  team-ai request id and model version into the warehouse. (seller-forecast-card)

  Scenario: Seller with history sees the forecast
    Given a seller with a paid order line and a stored forecast of 14 days for that listing
    And the seller is logged in the browser
    When the seller loads the analytics page
    Then the forecast card lists the listing with 14 rows of p10, p50 and p90 exactly as served
    And the forecast card shows the reorder point of the served forecast rounded up
    And the forecast card shows the served model version and the run date as dd/mm/yyyy

  Scenario: Listing without a forecast key
    Given a seller with a paid order line for a listing that has no forecast key
    And the seller is logged in the browser
    When the seller loads the analytics page
    Then the forecast card says "Chưa đủ dữ liệu để dự báo"
    And the forecast card shows no forecast numbers, reorder point or model version

  @destructive
  Scenario: Store unavailable
    Given a seller with a paid order line and a stored forecast of 14 days for that listing
    And the seller is logged in the browser
    And the forecast store is stopped
    When the seller loads the analytics page
    Then the analytics page answers 200
    And the forecast card says "Dự báo tạm thời không khả dụng"
    And the forecast card shows no forecast numbers, reorder point or model version
    And the analytics KPI row is still shown

  Scenario: Gateway rejects a foreign seller id
    Given a seller with a paid order line and a stored forecast of 14 days for that listing
    And another seller exists
    When the other seller calls GetDemandForecast for the first seller's listing through the gateway
    Then the forecast call is denied with "permission_denied" and carries no forecast numbers

  @needsSeller
  Scenario: Provenance reaches the warehouse
    Given a seeded seller is logged in
    When the seller opens the new listing form for Magic Listing
    And the seller generates a Magic Listing draft
    Then the warehouse holds an ai_interaction event of that seller whose model_version is the one team-ai serves and whose impression_id is not empty
