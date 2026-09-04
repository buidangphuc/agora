# [W1] F5 Bundle deals (team-domain)

## Role
SE

## Objective
Implement the RPC(s) below in **team-domain** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-domain/)
- team-domain/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: listing CRUD (Create/Get/List)

## Contracts you implement
`CreateBundle(title, listing_ids, bundle_price)`, `GetBundle(id)`, `ListBundlesBySeller(seller_id, page)`.

## Reference implementation
Mirror **listing CRUD (Create/Get/List)**: same handler/service/repository layering + test layout. Table: bundles(id, seller_id, title, listing_ids text[], bundle_price int64, created_at). Owner-scoped create.

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-domain && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
