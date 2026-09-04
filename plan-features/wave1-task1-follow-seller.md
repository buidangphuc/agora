# [W1] F1 Follow Seller (team-engagement)

## Role
SE

## Objective
Implement the RPC(s) below in **team-engagement**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-engagement/)
- team-engagement/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: ListFavorites (favorites graph + list) in internal/{handler,service,repository}

## Contracts you implement
`FollowSeller`, `UnfollowSeller`, `ListFollowedSellers`, `IsFollowing`, `ListFollowedListings` (feed of listings from followed sellers, paginated).

## Reference implementation
Mirror **ListFavorites (favorites graph + list) in internal/{handler,service,repository}** in team-engagement: same handler/service/repository layering, same test layout, new domain. Migration: follows(user_id, seller_id, created_at) unique.

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-engagement && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
