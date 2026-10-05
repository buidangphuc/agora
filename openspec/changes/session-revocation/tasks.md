## 1. Contract — platform-core

- [ ] 1.1 Add `SessionRevoked { session_id, user_id, expires_at }` (identity proto) additively; verify `buf lint` and `buf breaking` against feat/ui-system, vendor to team-identity and team-gateway, regenerate
- [ ] 1.2 Provision the `identity.events` topic (redpanda-init in compose) and document it in `platform-core/docs` and an ADR-0003 addendum; verify the compose config renders

## 2. Code — team-identity

- [ ] 2.1 Migration for `identity_outbox_events` and an outbox store with the ordered claim query; verify the Postgres ordering test pattern (skips without `TEST_DATABASE_URL`) and `go test ./...`
- [ ] 2.2 `RevokeSession` updates the session and inserts the outbox row in one transaction; verify a test that a failed outbox write rolls back the revoke
- [ ] 2.3 Relayer to `identity.events` started in main (`KAFKA_ENABLED`, `OUTBOX_*` env, `.env.example`, compose env); verify relayer unit tests and the env-example sync test

## 3. Code — team-gateway

- [ ] 3.1 `identity.events` consumer (no shared group, earliest on start) feeding an in-memory `sid` denylist with expiry pruning; verify unit tests for add, expiry, replay and duplicate events
- [ ] 3.2 The auth path rejects a denylisted `sid` with 401 (`WWW-Authenticate: Bearer error="invalid_token"`); verify bearer tests for a revoked sid on a public and a protected route
- [ ] 3.3 `outgoing()` sets `x-client-ip` / `x-client-user-agent` (clipped), `X-Forwarded-For` only from `TRUSTED_PROXIES`; verify tests for direct, trusted-proxy and spoofed inputs
- [ ] 3.4 Run `auth-scope-reviewer` and `contract-boundary-reviewer` over the diff; verify no BLOCKING finding

## 4. E2E — platform-e2e

- [ ] 4.1 Scenarios: revoked token gets 401 within 5 s; revocation survives a gateway restart (`@destructive`); new session shows device and IP; spoofed forwarded IP is ignored; FEATURES.yaml entries; verify green against the agora stack and flip to `automated`
- [ ] 4.2 Run `openspec validate session-revocation --strict`; verify it is valid
