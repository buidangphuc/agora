## 1. Code track: team-identity

- [ ] 1.1 Failing tests then migration: users columns `seller_terms_version`, `seller_terms_accepted_at`; `SELLER_TERMS_VERSIONS` config (fail fast when empty). Scenarios: acceptance is recorded, unknown terms version, terms version must match exactly
- [ ] 1.2 Store `GrantSeller(userID, version, now, enqueue)`: one tx, row lock, re-read status ACTIVE, idempotent for seller, outbox row only on first grant. Scenarios: first grant emits one event, failed transaction leaves no event, concurrent calls emit one event, admin restriction not bypassed, second call idempotent
- [ ] 1.3 `AuthService.BecomeSeller`: validate version, session live check, sign token on same `sid` with `ScopesForUser`, no refresh rotation, no audit. Scenarios: buyer becomes seller, refresh token unchanged, non-ACTIVE refused, existing seller non-ACTIVE gets no token, revoked session, no audit entry
- [ ] 1.4 `events.BuildUserRoleGranted` envelope and rpc handler `BecomeSeller` (`auth.RequireUser`, status mapping INVALID_ARGUMENT / PERMISSION_DENIED / UNAUTHENTICATED). Scenario: first grant emits one event (app outbox test)
- [ ] 1.5 `go test ./...`, vet, lint in team-identity

## 2. Code track: team-gateway

- [ ] 2.1 Route `AuthService/BecomeSeller` for user principals only; not in the public list; forward `sid`. Scenarios: service or anonymous refused, signed-in buyer through the gateway
- [ ] 2.2 gateway tests, vet, lint

## 3. Docs

- [ ] 3.1 Add `SELLER_TERMS_VERSIONS` to `team-identity/README.md`

## 4. E2E track

- [ ] 4.1 `platform-e2e/tests/e2e/features/identity/seller_onboarding.feature`: signed-in buyer becomes seller through gateway, admin restriction after onboarding, anonymous refused, unknown, older and whitespace/case terms versions `INVALID_ARGUMENT`, existing non-ACTIVE seller denied, second call idempotent (event count via outbox/Kafka consumer), latency over 50 calls
- [ ] 4.2 `openspec validate identity-seller-onboarding --strict`
