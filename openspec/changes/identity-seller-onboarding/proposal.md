## Why

G15: a buyer who opens the seller app hits a dead end ("Cần tài khoản Người Bán" without a button) and cannot become a seller; there is no RPC to grant the seller role. Locked decision (initiative p10-backend-gaps, 2026-11-02): the buyer accepts the seller terms in the seller app and is a seller immediately.

initiative: p10-backend-gaps

## What Changes

- team-identity implements `AuthService.BecomeSeller(terms_version)` (contract from `p10-gaps-contract`) for an authenticated ACTIVE user: adds role `seller`, stores `seller_terms_version` and `seller_terms_accepted_at`, issues a new access token on the caller's session (same `sid`, refresh token unchanged) carrying seller scopes, and enqueues `UserRoleGranted` on `identity.events` through the existing outbox in the same transaction as the role write.
- Idempotent for an ACTIVE existing seller: fresh token, no write, no event, original acceptance record kept.
- `INVALID_ARGUMENT` unless `terms_version` exactly equals the current (first) `SELLER_TERMS_VERSIONS` entry (older listed versions, whitespace or case variants are refused); `PERMISSION_DENIED` for any non-ACTIVE account (RESTRICTED included), also for an existing seller who is no longer ACTIVE (no token); `UNAUTHENTICATED` for no user principal or a revoked/unknown session.
- No audit entry: self-service, locked decision.
- team-gateway exposes `BecomeSeller` to authenticated users (any signed-in user scope, not public, not service); admin can still restrict the new seller through `AccountAdminService.SetUserStatus`.
- Seller-app UI (onboarding screen, layout change, `login-enforcement-ui`, `login-return`, `order-lifecycle-ui` modifications) belongs to slice `seller-onboarding-ui` (owner team-frontend-seller), not this change.

## Capabilities

### New Capabilities
- `identity-seller-onboarding`: BecomeSeller behaviour, event, gateway exposure.

### Modified Capabilities
none

## Impact

`team-identity/internal/identity/auth.go` (hot file shared with identity-sso-handoff), `internal/rpc`, `internal/store` (users columns + migration, role grant with outbox), `internal/events`; `team-gateway/internal/edge/identity*.go` and method-scope table, `team-identity/README.md`.

## Non-goals

Admin approval queue, audit entry, domain seller-profile reaction to `UserRoleGranted` (consumers may react later), revoking the seller role, terms text management beyond a version string, un-restricting, any seller-app UI.

## NFR

- `BecomeSeller` p95 < 200 ms locally (one DB transaction plus one signature); measured by scenario "latency over 50 calls".
- Availability: depends on team-identity and its Postgres only; no audit or other upstream call, so no fail-closed audit path.
- Exactly one role write and one `UserRoleGranted` row per user: concurrent calls by one user yield one event (second sees seller, returns the fresh token).
- Outbox: event delivery at-least-once; the role write and the outbox row commit atomically or not at all.

## Data classification

- `seller_terms_version` (string) and `seller_terms_accepted_at` (timestamp) on the user row: purpose proof of acceptance of seller terms; basis contract; retention account lifetime; not logged; not PII beyond linkage to the user id.
- `UserRoleGranted{user_id, role, granted_at}` on `identity.events`: purpose downstream reaction to role change; basis contract; retention 7 days (Kafka topic default retention.ms = 604800000; no per-topic override exists in platform-core); user id pseudonymous, never logged raw (sloglint).
- Access token (bearer secret): returned once to the caller, never logged; the response carries no refresh token.

## Threat model

- Assets: the seller role (grants `listing.write`), seller terms acceptance proof, session tokens.
- Abuse cases: (1) a restricted/suspended/banned account regains seller power through onboarding -> refused `PERMISSION_DENIED` before any write; (2) caller upgrades another user -> the target is always the authenticated principal, the request has no user id; (3) service principal or anonymous call -> refused; (4) revoked session reused to mint a token -> `UNAUTHENTICATED`, no write; (5) bogus terms version to skip acceptance -> `INVALID_ARGUMENT`; (6) replay/double-click -> idempotent, one event.
- Authz rule: caller must be an authenticated user principal whose account is ACTIVE; the role granted is only `seller` for the caller. Admin keeps `SetUserStatus` to restrict afterwards.
