@search @hybrid @needsModelserveOverlay
Feature: Hybrid retrieval index-time vectors
  The indexer vectorizes listings through the model server and fails open. The read-model document
  is read straight from OpenSearch (the same `_doc` reader the tombstone scenarios use) because the
  gateway does not expose vectors. Change add-hybrid-retrieval-platform, spec search-retrieval.

  Scenario: Published listing receives embedding at index time
    When a seller publishes a listing with a unique title through the gateway
    Then the read-model document holds a 384 dimension embedding that is the model server's vector of its text
    And the document is not marked vector_pending

  Scenario: Embedding failure marks vector_pending without DLQ
    When a seller publishes a listing whose title carries a failing-embedding directive
    Then the listing is indexed and searchable with vector_pending true and no embedding
    And no record of the listing is parked on the listing events DLQ

  Scenario: Fine-grained price or status update preserves existing embedding
    Given a published listing whose read-model document is embedded
    When a ListingPricingChanged with a new price and a newer occurred_at is published to listing.events
    Then the document shows the new price and the same embedding, still not vector_pending

  Scenario: Version guard prevents stale events during re-indexing
    Given a published listing whose read-model document is embedded
    When a ListingChanged UPDATED with another title and an occurred_at older than the creation is published to listing.events
    Then the read-model document keeps its title and embedding

  @destructive @slow
  Scenario: Replaying listing events generates dense embeddings
    Given a published listing whose read-model document is embedded
    When a second indexer with a new consumer group replays listing.events onto a new index
    Then the new index holds that listing with its text and a 384 dimension embedding
