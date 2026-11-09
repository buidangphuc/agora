@search @analytics
Feature: Search responses carry a request id and a model version that the beacons and the report use
  SearchListings and RunSavedSearch return a fresh request_id and a model_version naming the
  retrieval path that ran. The /search page passes both to its impression and click beacons and the
  admin GetSearchPerformance report groups them per model version. Scenarios are named after the
  `#### Scenario:` titles of change search-attribution (spec search-attribution). Scenarios tagged
  needsModelserveOverlay need platform-e2e/compose/modelserve.override.yaml. The ranker version
  override, the engine-less handler, the failing index, team-ai and the empty request_id page are
  unit-verified.

  Scenario: A search returns a request id
    Given a published listing whose title carries a unique keyword is searchable
    When a buyer searches for that keyword through the gateway
    Then the response request_id matches 32 lowercase hex characters

  Scenario: Paging gets a new request id
    Given two published listings share a unique keyword and are searchable
    When a client searches for that keyword with page size 1 and then with the next_cursor of page 1
    Then the two responses carry different request_id values

  Scenario: Repeating one search gets a new request id
    Given a published listing whose title carries a unique keyword is searchable
    When a client searches for that keyword 1000 times with identical parameters
    Then the 1000 responses carry 1000 distinct request_id values

  Scenario: A saved search run returns a request id
    Given a published listing whose title carries a unique keyword is searchable
    And the buyer owns a saved search for that keyword
    When the owner runs the saved search twice
    Then each run response request_id matches 32 lowercase hex characters
    And the two run responses carry different request_id values

  Scenario: A saved search of another user is rejected
    Given a published listing whose title carries a unique keyword is searchable
    And the buyer owns a saved search for that keyword
    When another user runs that saved search
    Then the call fails with NOT_FOUND and no request_id is returned

  Scenario: An anonymous caller can still search
    Given a published listing whose title carries a unique keyword is searchable
    When a caller with no principal searches for that keyword through the gateway
    Then the response carries a valid request_id and model_version

  @hybrid @needsModelserveOverlay
  Scenario: A hybrid search reports hybrid
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword with search mode "SEARCH_MODE_HYBRID"
    Then the model_version equals "hybrid@v1"

  @hybrid @needsModelserveOverlay
  Scenario: The reranker is reported
    Given three embedded listings share a unique keyword
    When a buyer searches for that keyword with search mode "SEARCH_MODE_HYBRID" and the directive "[[fake rerank=reverse]]"
    Then the model_version equals "hybrid+rerank@v1"

  @hybrid @needsModelserveOverlay
  Scenario: A reranker failure is not reported as rerank
    Given three embedded listings share a unique keyword
    When a buyer searches for that keyword with search mode "SEARCH_MODE_HYBRID" and the directive "[[fake rerank_status=500]]"
    Then the model_version equals "hybrid@v1"

  @hybrid @needsModelserveOverlay
  Scenario: A fallback page never carries rerank
    Given three embedded listings share a unique keyword
    When a buyer searches for that keyword with search mode "SEARCH_MODE_HYBRID" and the directive "[[fake status=500]]"
    Then the model_version equals "lexical@v1"
    And the model_version does not contain "+rerank"

  Scenario: A lexical request reports lexical
    Given a published listing whose title carries a unique keyword is searchable
    When a buyer searches for that keyword with search mode "SEARCH_MODE_LEXICAL"
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: Semantic fallback reports lexical
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword with search mode "SEARCH_MODE_SEMANTIC" and the directive "[[fake status=500]]"
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: Hybrid with a failed semantic leg reports lexical
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword with search mode "SEARCH_MODE_HYBRID" and the directive "[[fake status=500]]"
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: A key sort in hybrid mode reports lexical
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword in hybrid mode sorted by price ascending
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: Deep paging beyond the fusion window reports lexical
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword in hybrid mode with the cursor 250
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: An empty query reports lexical
    When a buyer searches with an empty query in hybrid mode
    Then the model_version equals "lexical@v1"

  @hybrid @needsModelserveOverlay
  Scenario: The unspecified mode resolves to the mode that ran
    Given a published listing with a unique keyword is embedded and searchable
    When a buyer searches for that keyword without a search mode
    Then the model_version equals "hybrid@v1"

  Scenario: An unset ranker version defaults to v1
    Given a published listing whose title carries a unique keyword is searchable
    When a buyer searches for that keyword with search mode "SEARCH_MODE_LEXICAL"
    Then the model_version ends with "@v1"

  Scenario: A saved search run reports lexical
    Given a published listing whose title carries a unique keyword is searchable
    And the buyer owns a saved search for that keyword
    When the owner runs the saved search twice
    Then each run response model_version equals "lexical@v1"

  Scenario: An admin through the gateway receives search_results rows
    Given the seeded admin for search performance
    When a visitor posts a search_results impression under a fresh model version
    Then every row of the admin's search performance report has placement_id "search_results" and the fresh model version has a row

  Scenario: window_hours 169 is rejected by team-analytics
    Given the seeded admin for search performance
    When the admin calls GetSearchPerformance with window_hours 169
    Then the gateway answers HTTP 400 with code invalid_argument for search performance

  @buyer
  Scenario: Impression and click carry the identity
    Given 3 listings are indexed under one keyword for the search page
    When the buyer opens the search results for that keyword recording the beacons
    And the buyer clicks the first search result
    Then the search_results impression beacons and the click beacon carry one 32 hex impression_id, one model_version and placement_id "search_results"

  @buyer
  Scenario: Re-sorting gets a new impression id
    Given 3 listings are indexed under one keyword for the search page
    When the buyer opens the search results for that keyword recording the beacons
    And the buyer picks the sort "Mới nhất"
    Then the second page of search_results beacons carries an impression_id different from the first page's

  @buyer
  Scenario: Paging gets a new impression id
    Given 26 listings are indexed under one keyword for the search page
    When the buyer opens the search results for that keyword recording the beacons
    And the buyer moves to page 2 of the results
    Then the second page of search_results beacons carries an impression_id different from the first page's

  @buyer @hybrid @needsModelserveOverlay
  Scenario: A fallback is labelled with the mode that ran
    Given 3 listings are indexed under one keyword for the search page
    When the buyer opens the search results for that keyword with a failing embedding directive recording the beacons
    Then the search_results beacons carry a model_version that starts with "lexical@"

  @buyer
  Scenario: An empty result sends no impression
    Given 3 listings are indexed under one keyword for the search page
    When the buyer searches the page for a term with no matches recording the beacons
    Then the page renders the empty state and sends no search_results impression beacon
    And a following search for the indexed keyword sends impression beacons carrying an impression_id
