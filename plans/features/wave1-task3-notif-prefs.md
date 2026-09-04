# [W1] F3 Notification preferences + digest (team-notification)

## Role
SE

## Objective
Implement the RPC(s) below in **team-notification**, end-to-end (handler → service → repository + migration), with unit tests. Verify green on host.

## Write-set (EXCLUSIVE — nothing outside team-notification/)
- team-notification/** (its internal/, cmd/, migrations/, vendored proto/, regenerated stubs, *_test.go)

## Read-only dependencies
- platform-core proto for this domain (authored in Wave 0)
- Reference implementation in this repo: the alerts subscription handler/service/repository (SubscribeAlert etc.)

## Contracts you implement
`GetNotificationPrefs`, `UpdateNotificationPrefs` (per-channel/per-type toggles + digest frequency). Digest = a scheduler that bundles pending notifications per frequency.

## Reference implementation
Mirror **the alerts subscription handler/service/repository (SubscribeAlert etc.)** in team-notification: same handler/service/repository layering, same test layout, new domain. Migration: notification_prefs(user_id, prefs jsonb, digest_freq).

## Acceptance criteria
- [ ] RPCs implemented with real DB logic (not stubs); auth-scoped where user-owned
- [ ] Anonymous / empty-input handled without panics
- [ ] >=3 unit tests (happy + 2 edges incl. cross-user isolation where applicable)
- [ ] Verify command green

## Review (different agent)
SE rubric + auth-scope-reviewer + contract-boundary-reviewer (new authed RPC / boundary) → require CLEAN.

## Verify
export PATH=/opt/homebrew/bin:$PATH; cd team-notification && (make proto || buf generate) && go test ./...

## Out of scope
- Do NOT touch team-gateway, team-frontend, or any other repo (forwarders + UI = integration wave).
- Do NOT edit platform-core proto (Wave 0 did it) or hand-edit generated code. Money stays mock.
