# [W1-T3] team-search — facet aggregations + price-range facet

## Role
SE   # read-model, OpenSearch aggregations

## Objective
SearchListings returns `facets` (category, price_ranges, ratings, sellers with counts) computed from OpenSearch aggregations alongside hits. Existing request filters (min_price/max_price/min_rating/category_id) already work; wire them to also drive facet counts.

## Write-set (EXCLUSIVE)
- team-search/internal/**   (edit — handler maps Facets; service/repository add aggregation query; + _test.go)

## Read-only dependencies
- team-search generated stubs (Facets from Wave 0)
- existing team-search SearchListings impl
- 00-spec.md §Contracts F2

## Contracts
Add to SearchListingsResponse: `Facets facets = 3`. price_ranges = fixed buckets (e.g. 0-100k, 100k-500k, 500k-1M, 1M+), key = label, count from OpenSearch range agg. ratings = terms agg on rating (>=4, >=3...). categories/sellers = terms agg, key = id.

## Acceptance criteria
- [ ] Response includes non-empty facets when matching docs exist; counts match filtered result set.
- [ ] Applying a facet filter (e.g. category_id) narrows hits AND recomputes other facets.
- [ ] Empty result → empty facet buckets, never nil-panic.
- [ ] gofmt/go vet/go test clean; test asserts agg mapping with a seeded/mock index or unit-tests the query builder.

## Review (gate — different agent)
Peer agent against SE rubric (experts.md).

## Verify
```bash
# in Docker: (cd team-search && gofmt -l . && go vet ./... && go test ./...)
```

## Out of scope
- Do NOT change proto/generated. No semantic/vector ranking (out of scope). No gateway/frontend.
