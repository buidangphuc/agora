# [W1] F4 Loyalty check-in (team-engagement)

## Role
SE

## Objective
Implement the RPC(s) below in **team-engagement** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-engagement/)
- team-engagement/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: ListFavorites / RecordView (handler+repo)

## Contracts you implement
`CheckIn()` -> {streak, coins_earned}, `GetLoyalty()` -> {streak, coin_balance, last_checkin}.

## Reference implementation
Mirror **ListFavorites / RecordView (handler+repo)**: same handler/service/repository layering + test layout. Tables: loyalty_accounts(user_id pk, coin_balance, streak, last_checkin), checkins(user_id, day unique). Idempotent per-day check-in; streak resets if a day is skipped.

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-engagement && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
