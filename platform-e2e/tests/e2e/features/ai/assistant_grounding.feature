@ai @buyer @needsModelserveOverlay
Feature: Shopping assistant grounded in live catalog search
  The assistant builds its product cards from team-search hits hydrated with team-domain listings, so a
  card exists only for a published in-stock listing and carries the domain's title and price. Everything
  goes through the gateway (Connect JSON). Needs team-ai with ASSISTANT_BACKEND=mock, team-search and
  team-domain (change ai-assistant-grounding). The destructive scenarios stop team-search or recreate
  team-ai with compose/rag-off.override.yaml and restore the standing stack in teardown.

  Scenario: A published in-stock listing is returned as a grounded card
    When a seller publishes a listing with stock 5 and a buyer asks the assistant for its unique title
    Then the response has a product card whose listing id, title and price equal that listing's domain values

  Scenario: An out-of-stock listing is not returned
    Given the assistant returns a published in-stock control listing
    And a published listing with stock 0 shares the control listing's title word
    When a buyer asks the assistant for that title word
    Then the response contains no product card for the listing with stock 0 and still has the control card

  Scenario: A listing unpublished in domain disappears from the assistant
    Given the assistant returns a published in-stock control listing and a second published listing
    When the seller changes the second listing to draft
    Then the assistant no longer returns the second listing and still returns the control listing

  Scenario: A budget in the question limits the prices
    Given published listings priced 300000 and 800000 share a unique title word
    When a buyer asks the assistant for that word with the budget "dưới 500k"
    Then every returned card has a price of at most 500000 and the 300000 listing is returned

  Scenario: The mock reply is the template over the real cards
    Given the assistant returns a published in-stock control listing
    When a buyer asks the assistant for that title word twice
    Then both replies are byte-identical and name the first card's title and its price

  Scenario: A question with no match returns no cards
    When a buyer asks the assistant for a product no published in-stock listing matches
    Then the response has no product cards and a non-empty reply naming no listing title

  Scenario: The response carries request id and model version
    Given the assistant returns a published in-stock control listing
    When a buyer asks the assistant for that title word
    Then the request id is 32 lowercase hex characters and the model version matches "mock:assistant-v1|" followed by the search model version

  @destructive
  Scenario: The assistant answers when search is down
    Given the assistant returns a published in-stock control listing
    And team-search is stopped
    When a buyer asks the assistant for that title word
    Then the call succeeds with the retrieval-failed reply, no product cards and the model version "mock:assistant-v1"
    And the control card comes back once team-search is restored

  @destructive
  Scenario: Without the RAG store the assistant still returns real listings only
    Given a published in-stock control listing exists in the catalog
    And team-ai runs with RAG disabled
    When a buyer asks the assistant for a t-shirt
    Then every returned card id is a listing that exists in team-domain
