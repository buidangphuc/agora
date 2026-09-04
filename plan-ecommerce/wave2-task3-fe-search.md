# [W2-T3] frontend — faceted search filters UI

## Role
SE

## Objective
Search results page shows facet filters (category, price range, rating, seller) with counts; selecting a facet narrows results and updates the URL query.

## Write-set (EXCLUSIVE)
- team-frontend/src/app/search/**          (edit)
- team-frontend/src/features/search/**     (edit)
- team-frontend/src/lib/gateway/search.ts  (create/edit — server-only client mapping facets)

## Read-only dependencies
- team-frontend/src/generated/** (search stubs w/ Facets, Wave 0)
- existing search page + src/app/api/suggest
- 00-spec.md §Contracts F2

## Contracts
Gateway module returns hits + facets. Facet selection maps to request filters (category_id, min_price/max_price, min_rating). URL query is the source of truth for active filters (shareable).

## Acceptance criteria
- [ ] Facet sidebar renders buckets with counts; empty state safe.
- [ ] Selecting/deselecting a facet updates results and the URL; counts refresh.
- [ ] Typechecks/build pass; gateway-only data access.

## Review (gate — different agent)
Peer agent against SE rubric.

## Verify
```bash
# (cd team-frontend && npm run typecheck && npm run build)
```

## Out of scope
- Do NOT edit other features' dirs or shared shells. No client-side call to team-search directly.
