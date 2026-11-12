@search
Feature: Search listing stock lookup
  As a service such as team-ai
  I want to read the live projected stock of up to 10 listings by id
  So that I can drop sold-out candidates without running a search

  Scenario: Stock is returned in request order
    Given published listings A with stock 5, B with stock 0 and C with stock 12 in the read-model
    When a service calls GetListingStock with [C, A, B]
    Then the response has exactly three entries [C: 12, A: 5, B: 0], with B's stock present as 0

  Scenario: A checkout is visible in the lookup
    Given a published listing with stock 1 that appears in search
    When a buyer checks out quantity 1 of it through the gateway
    Then within 30 seconds GetListingStock for that listing returns stock 0

  Scenario: An unknown listing has no stock
    Given listing A with stock 5 and an id U that was never indexed
    When a service calls GetListingStock with [A, U]
    Then the response has two entries, A with stock 5 and U with stock absent

  Scenario: A deleted listing has no stock
    Given a listing that was indexed with stock 5 and then deleted
    When a service calls GetListingStock with its id
    Then the response has one entry for it with stock absent

  Scenario: Ten ids is accepted, eleven is rejected
    When a service calls GetListingStock with exactly 10 ids, then with 11 ids
    Then the first call returns 10 entries and the second fails with invalid_argument

  Scenario: An empty or malformed request is rejected
    When a service calls GetListingStock with no ids, then with ["A", ""]
    Then both calls fail with invalid_argument

  Scenario: A user or anonymous caller is refused
    Given listing A with stock 5 and an id U that was never indexed
    When an anonymous caller, then a user principal with search:read, then a service principal without search:read, calls GetListingStock with one valid id
    Then the first fails with unauthenticated, the second and third with permission_denied, and no response carries stock

  @destructive
  Scenario: The gateway does not expose the lookup
    When a client sends an HTTP request to the gateway for any path derived from SearchService/GetListingStock
    Then the gateway answers HTTP 404 for every path
    And team-search records no GetListingStock call

  @destructive
  Scenario: An unreachable index fails the call
    Given the search index is unreachable
    When a service calls GetListingStock with one valid id
    Then the call fails with unavailable and returns no entries

  @destructive
  Scenario: A slow index fails the call within the deadline
    Given the search index answers slower than 200 ms
    When a service calls GetListingStock with one valid id
    Then the call fails with unavailable within 400 ms and returns no entries

  Scenario: Requested ids are not logged
    Given listing A with stock 5 and an id U that was never indexed
    When a service calls GetListingStock with ids [A, U]
    Then the captured team-search log lines for that call contain the id count 2 and neither id value

  Scenario: A 10-id lookup is fast
    Given the mainx stack with the OpenSearch overlay and 10 indexed listings
    When a service makes 200 sequential GetListingStock calls with those 10 ids
    Then the p95 latency is at most 20 ms
