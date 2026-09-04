# [W3-T2] e2e — faceted search filters

## Role
QA

## Objective
Automated e2e proving the search results page shows facets with counts and that selecting a facet narrows results. Flip search-facets status to automated.

## Write-set (EXCLUSIVE)
- platform-e2e/tests/e2e/features/frontend/search_facets.feature   (create)
- platform-e2e/tests/e2e/step_definitions/search_sort_steps.py     (edit — add facet steps)
- platform-e2e/src/pages/search_page.py                            (edit)
- team-search/FEATURES.yaml                                        (edit — status: automated)

## Read-only dependencies
- existing search_sort_steps + search_page patterns
- 00-spec.md §Contracts F2

## Contracts / scenarios
- Given indexed listings across categories/prices, When buyer opens search, Then facet buckets show counts.
- When buyer selects a price-range facet, Then results narrow and other facet counts update and the URL reflects the filter.

## Acceptance criteria
- [ ] Scenarios pass against local stack; steps fully wired (collect green).
- [ ] team-search/FEATURES.yaml flipped to automated.

## Review (gate — different agent)
Peer agent against QA rubric.

## Verify
```bash
docker compose -p platform-core up -d
make -C platform-e2e collect
python -m pytest platform-e2e/tests/e2e -k "search_facets or facet" -q
```

## Out of scope
- No other feature dirs/step files. Do not edit shared slot locators (Wave 0).
