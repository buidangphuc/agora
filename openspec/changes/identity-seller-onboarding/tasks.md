## 1. Code track: team-identity

- [ ] 1.1 Failing tests then migration: users columns `seller_terms_version`, `seller_terms_accepted_at`; `SELLER_TERMS_VERSIONS` config (fail fast when empty). Scenarios: acceptance is recorded, unknown terms version, terms version must match exactly
- [ ] 1.2 Store `GrantSeller(userID, version, now, enqueue)`: one tx, row lock, re-read status ACTIVE, idempotent for seller, outbox row only on first grant. Scenarios: first grant emits one event, failed transaction leaves no event, concurrent calls emit one event, admin restriction not bypassed, second call idempotent
- [ ] 1.3 `AuthService.BecomeSeller`: validate version, session live check, sign token on same `sid` with `ScopesForUser`, no refresh rotation. Scenarios: buyer becomes seller, refresh token unchanged, non-ACTIVE refused, existing seller non-ACTIVE gets no token, revoked session
- [ ] 1.4 `events.BuildUserRoleGranted` envelope and rpc handler `BecomeSeller` (`auth.RequireUser`, status mapping INVALID_ARGUMENT / PERMISSION_DENIED / UNAUTHENTICATED). Scenario: first grant emits one event (app outbox test)
- [ ] 1.5 `go test ./...`, vet, lint in team-identity

- [x] 1.6 Audit `auth.become_seller`, fail-closed. Write-set: `team-identity/internal/identity/**`, `team-identity/internal/store/**`, `team-identity/internal/rpc/**`, `team-identity/internal/app/**`. Red tests first: `TestBecomeSellerWritesAuditEntry`, `TestBecomeSellerAuditDownRefusesGrant`, `TestBecomeSellerRepeatDoesNotAudit` (team-identity/internal/identity), `TestGrantSellerAuditFailureRollsBack` (team-identity/internal/store). First grant: audit write inside the `GrantSeller` transaction before commit, metadata `{terms_version, result:"granted"}` only; failure returns `UNAVAILABLE` with `ErrorInfo{identity.platform, AUDIT_UNAVAILABLE}` via the rpc status mapping; repeat by an existing seller skips audit. Scenarios: a first grant writes one audit entry, audit unavailable refuses the grant, an audit failure after the role write rolls it back, a repeat call by a seller does not audit again
- [x] 1.7 Contract edit (build agent; belongs to `p10-gaps-contract` files, not this slice's write-set): list `auth.become_seller` in the `Action catalogue` comment of `WriteAuditEventRequest` in `platform-core/packages/proto/platform/audit/v1/audit.proto`, change the header to `Action catalogue (25)`, update `platform-core/packages/proto/gaps_contract_docs_test.go` (header string, audit_catalogue test action list and its name) and the vendored copies. Scenario: a first grant writes one audit entry (action name is catalogued)

## 2. Code track: team-gateway

- [ ] 2.1 Route `AuthService/BecomeSeller` for user principals only; not in the public list; forward `sid`. Scenarios: service or anonymous refused, signed-in buyer through the gateway
- [ ] 2.2 gateway tests, vet, lint
- [x] 2.3 Confirm the gateway passes upstream `UNAVAILABLE` + `ErrorInfo` `AUDIT_UNAVAILABLE` from `BecomeSeller` through `toConnectErr` unchanged (the voucher slice already adds this mapping; add `BecomeSeller` coverage only if the mapping is per-RPC). Write-set: `team-gateway/internal/edge/auth.go`, its test. Scenario: audit unavailable is refused through the gateway

## 3. Docs

- [ ] 3.1 Add `SELLER_TERMS_VERSIONS` to `team-identity/README.md`

## 4. E2E track

- [ ] 4.1 `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature`: signed-in buyer becomes seller through gateway, admin restriction after onboarding, anonymous refused, unknown, older and whitespace/case terms versions `INVALID_ARGUMENT`, existing non-ACTIVE seller denied, second call idempotent (event count via outbox/Kafka consumer and exactly one `auth.become_seller` audit entry), first grant writes one audit entry (query via admin `QueryAuditLog`), audit unavailable refused through the gateway (stop team-audit, expect `AUDIT_UNAVAILABLE`, user unchanged), latency over 50 calls
- [x] 4.1a Remove the e2e step asserting no audit entry from `seller_onboarding.feature` if present (scenario: a first grant writes one audit entry)
- [ ] 4.2 `openspec validate identity-seller-onboarding --strict`
