@buyer @search
Feature: Faceted search filters
  As a buyer
  I want the search results page to show facets with counts and let me filter by them
  So that I can narrow a large result set to the products I care about

  Background:
    Given seeded listings across categories and price ranges are indexed

  Scenario: Facet buckets render with counts
    When the buyer opens the search results for the seeded listings
    Then the facet sidebar shows category and price buckets with counts

  Scenario: Selecting a price-range facet narrows the results
    When the buyer opens the search results for the seeded listings
    And the buyer selects the "100000-500000" price-range facet
    Then the results narrow to the listings in that price range
    And the search URL reflects the selected price filter
    And the facet counts update to reflect the narrowed set

  Scenario: Filters are shareable URLs
    When the buyer opens the search results for the seeded listings
    And the buyer selects the "100000-500000" price-range facet
    Then the search URL reflects the selected price filter
    And opening the same search URL in a new tab shows the same selected facet

  Scenario: Active filters can be removed one by one
    When the buyer opens the search results for the seeded listings
    And the buyer selects the "100000-500000" price-range facet
    And the buyer removes the price filter tag
    Then the search URL no longer carries a price filter

  Scenario: Clear all filters
    When the buyer opens the search results for the seeded listings
    And the buyer selects the "100000-500000" price-range facet
    And the buyer clears all filters
    Then the search URL carries only the keyword

  Scenario: Sort options match what the server honours
    When the buyer opens the search results for the seeded listings
    Then the search results sort bar displays the sorting controls
    And the sort controls offer no Bán Chạy option

  Scenario: Page links preserve the query
    Given more than one page of listings is indexed
    When the buyer opens the newest-first search results for those listings
    Then the pagination links keep the query and the sort

  Scenario: Out-of-range page redirects
    Given more than one page of listings is indexed
    When the buyer opens a results page far beyond the last page
    Then the buyer is redirected to the last page of results

  Scenario: Zero results
    Given a keyword that matches no listing
    When the buyer opens the search results for that keyword
    Then an Empty state offers a clear-filters action linking to the search page

  # Server-side fetches cannot be intercepted from the browser: an invalid session cookie is
  # the deterministic way to make the frontend's SearchListings call fail.
  Scenario: Backend failure is not a silent empty
    Given a keyword that matches no listing
    And the search backend rejects the visitor's session
    When the buyer opens the search results for that keyword
    Then an error Alert with a retry link is shown instead of an empty result
