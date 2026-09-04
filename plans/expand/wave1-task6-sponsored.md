# [W1] F6 Sponsored listings (team-promotion)

## Role
SE

## Objective
Implement the RPC(s) below in **team-promotion** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-promotion/)
- team-promotion/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: the voucher service (create + list)

## Contracts you implement
`CreateAdCampaign(listing_id, budget, bid)` (mock credits), `ListSponsoredSlots(context)` -> sponsored listing_ids (active campaigns, bid-ordered).

## Reference implementation
Mirror **the voucher service (create + list)**: same handler/service/repository layering + test layout. Table: ad_campaigns(id, seller_id, listing_id, budget int64, bid int64, status, created_at). Mock — no real charge.

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-promotion && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
