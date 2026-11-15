@seller @needsSeller @ai
Feature: Seller chat copilot
  As a seller, AI copilot suggests draft replies to buyer messages.

  Scenario: Seller uses chat copilot to generate reply drafts
    Given a seller is in a buyer conversation
    When the seller invokes the copilot
    Then a suggested reply is drafted

  Scenario: Copilot does not claim stock for an out-of-stock listing
    Given the seller has a published listing with stock 0
    When the seller asks the copilot "còn hàng không?" about the listing
    Then no draft claims the item is available and at least one draft says it is out of stock

  Scenario: Copilot ignores another seller's listing
    Given another seller has a published listing with stock 5
    When the seller asks the copilot "còn hàng không?" about the other seller's listing
    Then no draft claims the item is available
    And no draft contains the other seller's listing title or id

  Scenario: Copilot makes no delivery-time promise
    Given the seller has a published listing with stock 5
    When the seller asks the copilot "giao hàng bao lâu vậy shop?" about the listing
    Then no draft states a delivery duration, cut-off hour or same-day dispatch
    And at least one draft asks the buyer for the delivery address

  Scenario: Copilot response carries request id and model version
    Given the seller has a published listing with stock 5
    When the seller asks the copilot "còn hàng không?" about the listing
    Then the response has a 32-character hex request id and the model version "rules-v1"

  Scenario: Copilot answers within the mock latency budget
    Given the seller has a published listing with stock 5
    When the seller makes 200 sequential copilot calls about the listing
    Then the 95th percentile latency is at most 300 ms
