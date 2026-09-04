# [W1] F2 Saved Searches (team-search)

## Role
SE

## Objective
Implement the RPC(s) below in **team-search** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-search/)
- team-search/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: SearchListings (handler + index) for RunSavedSearch; a simple repo for the saved list

## Contracts you implement
`SaveSearch(query, filters)`, `ListSavedSearches(page)`, `DeleteSavedSearch(id)`, `RunSavedSearch(id)` -> search results.

## Reference implementation
Mirror **SearchListings (handler + index) for RunSavedSearch; a simple repo for the saved list**: same handler/service/repository layering + test layout. Table: saved_searches(id, user_id, query, filters jsonb, created_at). RunSavedSearch reuses the existing search path.

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-search && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
