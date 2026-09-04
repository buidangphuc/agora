# [W1] F5 Seller storefront config (team-domain)

## Role
SE

## Objective
Implement the RPC(s) below in **team-domain**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-domain/)
- team-domain/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: listing CRUD (Create/Get listing) in internal/{handler,service,repository}

## Contracts you implement
`UpsertStorefront`, `GetStorefront` (by seller_id or slug): banner_url, tagline, featured_listing_ids, theme, slug.

## Reference implementation
Mirror **listing CRUD (Create/Get listing) in internal/{handler,service,repository}** in team-domain: same handler/service/repository layering, same test layout, new domain. Migration: storefronts(seller_id pk, slug unique, config jsonb).

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-domain && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
