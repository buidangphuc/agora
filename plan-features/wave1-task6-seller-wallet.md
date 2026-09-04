# [W1] F6 Seller wallet + payout (mock) (team-payment)

## Role
SE

## Objective
Implement the RPC(s) below in **team-payment**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-payment/)
- team-payment/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: existing payment handler/service/repository (CreatePayment/ProcessMockPayment)

## Contracts you implement
`GetWalletBalance`, `ListLedgerEntries` (paginated), `RequestPayout` (mock — records a PENDING payout ledger entry, no real money).

## Reference implementation
Mirror **existing payment handler/service/repository (CreatePayment/ProcessMockPayment)** in team-payment: same handler/service/repository layering, same test layout, new domain. Migration: wallet_ledger(id, seller_id, type, amount int64, status, created_at).

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-payment && buf generate && go test ./...

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
