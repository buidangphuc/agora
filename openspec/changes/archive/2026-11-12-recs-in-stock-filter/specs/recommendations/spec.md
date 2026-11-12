## MODIFIED Requirements

### Requirement: Recommend stays within the latency budget

The system SHALL answer `Recommend` within a serve-path budget of under 15ms (p99, excluding network to the caller), making at most one datastore round trip on the hot path — all business-rule fields except live stock read from the Qdrant payload or the Redis value — and SHALL cap the Qdrant path with `RECS_RETRIEVE_TIMEOUT_MS`, returning the popularity fallback on timeout rather than exceeding budget. The `GetListingStock` call of an `in_stock_only` placement is a service call, not a datastore round trip, and is not counted in that one; it covers at most `limit` x 2 ids capped at 20 (at most 2 concurrent calls), plus refill calls, all within the same `RECS_RETRIEVE_TIMEOUT_MS`. On a lookup error or timeout the placement SHALL be empty (fail closed) rather than fall back to the popularity list. The stock-lookup tests do not assert the 15 ms p99; it is covered by the existing serve-path measurement.

#### Scenario: Retrieval timeout yields a fallback, not an error

- **WHEN** the Qdrant retrieval exceeds `RECS_RETRIEVE_TIMEOUT_MS` on a placement without `in_stock_only`
- **THEN** `Recommend` returns the popularity fallback list instead of failing or blocking past the budget
- **VERIFIED BY**: `team-ai/tests/unit/test_recommend_servicer.py` `retrieval timeout yields popular fallback`

#### Scenario: Stock lookup timeout yields an empty placement, not an error

- **WHEN** the stock lookup of an `in_stock_only` placement exceeds `RECS_RETRIEVE_TIMEOUT_MS`
- **THEN** `Recommend` returns an OK response with zero items within `RECS_RETRIEVE_TIMEOUT_MS` + 50 ms and does not return the popularity fallback
- **VERIFIED BY**: `team-ai/tests/unit/test_in_stock_lookup.py` `timeout fails closed within budget`
