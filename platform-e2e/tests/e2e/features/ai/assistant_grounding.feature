@ai @buyer @needsModelserveOverlay
Feature: Shopping assistant grounded in indexed listings
  The assistant retrieves real listings from the RAG store the listing indexer feeds and returns them as
  product cards. Everything goes through the gateway (Connect JSON). Needs team-ai running the RAG store
  and the indexer (change assistant-rag-grounding, design.md "Deployment needs"); both are on in the base compose
  by default (change assistant-rag-default-on). The last scenario recreates team-ai with
  compose/rag-off.override.yaml and restores the standing configuration in teardown.

  Scenario: A published listing is returned as a product card
    When a seller publishes a listing with a unique title and a buyer asks the assistant for that title
    Then the response contains a product card whose listing id is that listing's id with its title and price

  Scenario: An unpublished listing disappears from the assistant
    Given the assistant returns a published control listing, a listing to unpublish and a listing to delete
    When the seller changes the first to draft and deletes the second
    Then the assistant no longer returns either of them and still returns the control listing

  @destructive
  Scenario: The assistant answers when retrieval is down
    Given the assistant returns a published listing
    And the modelserve router is stopped
    When a buyer asks the assistant for that listing
    Then the call succeeds with a non-empty reply and no product cards
    And the product card comes back once the router is restored

  @destructive
  Scenario: Without a RAG store the demo catalog answers
    Given team-ai runs with RAG disabled
    When a buyer asks the assistant for a t-shirt
    Then the response carries demo catalog cards and none of the listings published to the platform
