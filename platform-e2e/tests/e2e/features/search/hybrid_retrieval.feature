@search @buyer @hybrid
Feature: Multi-strategy Hybrid Retrieval Platform
  As a buyer
  I want search queries to retrieve products using both lexical keyword matching and semantic nearest neighbors
  So that I get relevant products even when terms differ or are phrasing variations

  Scenarios here are named after the `#### Scenario:` titles of change add-hybrid-retrieval-platform
  (spec search-retrieval). Those tagged @needsModelserveOverlay need
  platform-e2e/compose/modelserve.override.yaml (team-search -> modelserve router -> TEI fake).
  The RRF arithmetic, k and the weights are not observable through the gateway; those scenarios are
  exempt with a VERIFIED BY line in the delta spec (see design.md, "E2E verification").

  @needsBuyer @needsListing
  Scenario: Buyer searches catalog with multi-strategy hybrid retrieval
    Given a buyer is logged in
    When the buyer searches for "sneakers"
    Then the search results page displays matching products

  Scenario: Lexical strategy returns matching keyword candidates
    Given a published listing whose title carries a unique keyword is searchable
    When a buyer searches for that keyword in SEARCH_MODE_LEXICAL through the gateway
    Then the listing is among the hits with a positive score

  @needsModelserveOverlay
  Scenario: Semantic strategy returns dense vector nearest neighbors
    Given an embedded listing titled with a unique word and an embedded unrelated listing
    When a buyer searches for the semantic alias of that word in SEARCH_MODE_SEMANTIC
    Then the listing with that word is the first hit and the unrelated one ranks after it or is absent
    And a lexical search for the same alias finds neither listing

  @destructive @needsModelserveOverlay
  Scenario: Model server unavailability fails open
    Given a published listing with a unique keyword is embedded and searchable
    And the modelserve router is stopped
    When a buyer searches for that keyword in SEARCH_MODE_HYBRID through the gateway
    Then the search answers 200 with the listing among the hits

  @needsModelserveOverlay
  Scenario: Semantic strategy timeout degrades to lexical results
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword plus a slow-embedding directive in SEARCH_MODE_HYBRID
    Then the search answers 200 with the listing among the hits well before the embedding would finish
    And the TEI fake did receive the slow embedding request

  @needsModelserveOverlay
  Scenario: Degraded search responses maintain facet aggregations
    Given two embedded listings with a unique keyword differ in price
    When a buyer searches for that keyword plus a failing-embedding directive in SEARCH_MODE_HYBRID
    Then the TEI fake rejected the embedding of that query
    And the facet counts of the degraded response equal those of the lexical search and count both listings

  @needsModelserveOverlay
  Scenario: Unspecified search mode defaults to hybrid
    Given an embedded listing titled with a unique word
    When a buyer searches for the semantic alias of that word without a search mode
    Then the listing is among the hits although a lexical search for the alias finds nothing
    And the TEI fake received an embedding request for that query

  @needsModelserveOverlay
  Scenario: Explicit lexical mode skips vector embedding
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword plus a unique word in SEARCH_MODE_LEXICAL through the gateway
    Then the listing is among the hits
    And the TEI fake received no embedding request for that query
    And the same query in SEARCH_MODE_HYBRID does reach the TEI fake

  @needsModelserveOverlay
  Scenario: Explicit semantic mode queries vector index
    Given an embedded listing titled with a unique word
    When a buyer searches for the semantic alias of that word in SEARCH_MODE_SEMANTIC
    Then the listing is the first hit
    And the TEI fake received an embedding request for that query

  @needsModelserveOverlay
  Scenario: Reranker scores top candidates
    Given three embedded listings share a unique keyword
    When a buyer searches for that keyword in SEARCH_MODE_HYBRID, then again with a reverse-rerank directive
    Then the TEI fake received a rerank request listing those candidates
    And the three listings come back in the opposite order of the plain search

  @needsModelserveOverlay
  Scenario: Reranker failure falls back to RRF order
    Given three embedded listings share a unique keyword
    When a buyer searches for that keyword in SEARCH_MODE_HYBRID, then again with a failing-rerank directive
    Then the TEI fake answered the rerank request with a failure
    And the search answers 200 with the three listings in the RRF order of the same query's lexical and semantic legs

  @needsModelserveOverlay
  Scenario: Search within fusion window executes multi-strategy fusion
    Given an embedded listing matching a keyword lexically and an embedded listing matching its alias only semantically
    When a buyer searches for the keyword and the alias in SEARCH_MODE_HYBRID, first page
    Then both listings are among the hits although a lexical search finds only the first one
    And the TEI fake received an embedding request for that query

  @needsModelserveOverlay
  Scenario: Deep pagination beyond fusion window falls back to BM25
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword plus a unique word in SEARCH_MODE_HYBRID with the cursor 250
    Then the answer is 200 with the same total as the lexical search at that cursor
    And the TEI fake received no embedding request for that query
    And the same query in SEARCH_MODE_HYBRID at the first page does reach the TEI fake

  @needsModelserveOverlay
  Scenario: In-stock filter excludes a sold-out listing from semantic candidates
    Given an embedded in-stock listing and an embedded sold-out listing titled with the same unique word
    When a buyer searches for the semantic alias of that word in SEARCH_MODE_HYBRID with the in-stock filter
    Then the in-stock listing is among the hits and the sold-out listing is not

  @needsModelserveOverlay
  Scenario: Price range filter constrains semantic candidates
    Given an embedded cheap listing and an embedded dear listing titled with the same unique word
    When a buyer searches for the semantic alias of that word in SEARCH_MODE_HYBRID with a price range around the cheap one
    Then the cheap listing is among the hits and the dear listing is not

  @needsModelserveOverlay
  Scenario: Hybrid facets and total describe the fused candidate set
    Given an embedded listing matching a keyword lexically and an embedded listing matching its alias only semantically
    When a buyer searches for the keyword and the alias in SEARCH_MODE_HYBRID, first page
    Then the category facet counts sum to the response total and the total counts both listings
