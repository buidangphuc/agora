# [W1] F1 Account Safety Center (team-identity)

## Role
SE

## Objective
Implement the RPC(s) below in **team-identity** end-to-end (handler → service → repository + migration) with unit tests, green on host.

## Write-set (EXCLUSIVE — nothing outside team-identity/)
- team-identity/** (internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (Wave 0)
- Reference in this repo: an existing identity RPC (login/token handler + repository)

## Contracts you implement
`ListSessions` (active sessions/devices), `RevokeSession(session_id)`, `ListLoginHistory(page)`.

## Reference implementation
Mirror **an existing identity RPC (login/token handler + repository)**: same handler/service/repository layering + test layout. Tables: sessions(id, user_id, device, ip, created_at, last_seen, revoked), login_history(id, user_id, ip, ua, success, created_at). Self-scoped to principal.

## Acceptance criteria
- [ ] Real DB logic (InMemory + Postgres split so tests run without a live DB); auth-scoped where user-owned
- [ ] Anonymous/empty-input handled without panic
- [ ] >=3 tests (happy + 2 edges incl. cross-user isolation where relevant)
- [ ] Verify green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer → CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-identity && make proto && make test

## Out of scope
- Do NOT touch team-gateway, team-frontend, other repos (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0) or hand-edit generated code. Money stays mock.
