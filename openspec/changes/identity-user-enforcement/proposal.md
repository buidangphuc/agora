## Why

Today an admin has no way to act against an abusive account: `users` has no status, `AuthService.Login` accepts any correct password, and nothing ends a live session on demand except the user's own `RevokeSession`. The initiative spec (`plans/q6-trust-enforcement/00-spec.md`, user story 3) adds admin enforcement: restrict, suspend or ban a user from `/admin/users`. This slice builds the team-identity behaviour (status column, `AccountAdminService`, login and refresh refusal, restricted scope set, `UserStatusChanged` outbox event) and the team-gateway route and admin-scope policy. The proto contract (`AccountAdminService`, `UserStatus`, `AdminUser`, `UserStatusChanged`) is owned by `q6-trust-contract`; the frontend consumer is `frontend-user-enforcement`; the team-domain consumer that holds a banned seller's listings is `domain-seller-enforcement-hold`.

initiative: q6-trust-enforcement

## What Changes

- **team-identity**
  - Migration `0007_user_status` adds `status` (default `active`), `status_reason`, `status_until`, `status_changed_at` to `users`, plus an index on `(status)` for the `ListUsers` filter. Existing rows read as `active`.
  - New `AccountAdminService` (admin scope, enforced in the handler as well as at the edge):
    - `ListUsers`: read only, not audited. `query` is a case-sensitive username prefix or an exact user id; `status` filters (UNSPECIFIED = all); ordered by username ascending; page size default 20, max 100, cursor from `common.v1.PageRequest`. Never returns the password hash.
    - `SetUserStatus`: order is scope check, input validation, load user (`NotFound`), precondition (self or admin target: `FailedPrecondition`), team-audit write `user.set_status` (fail closed: any audit error, timeout or missing audit upstream returns `Unavailable` with no change), then one database transaction that updates the user row, revokes every active session of the user and enqueues outbox rows. `reason_code` must be lower_snake_case (`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`, 1 to 64 bytes); a non-matching code is `InvalidArgument` before any audit, as in the contract. Audit is written by team-identity as service principal `service-team-identity` holding `audit.write`.
  - Revocation inside the transaction (for target status RESTRICTED, SUSPENDED or BANNED): every non-revoked session is marked revoked with both refresh hashes cleared, one `SessionRevoked` outbox row per session (existing gateway denylist path, `expires_at` = now + access TTL), then exactly one `UserStatusChanged` outbox row (`identity.events`, key = user id). Re-setting the same RESTRICTED, SUSPENDED or BANNED status also revokes every session (decision A14). Setting ACTIVE revokes nothing: it stores the new `reason_code` in `status_reason`, clears `status_until`, and a restored user's scopes are re-derived at the next refresh or login (at most one access TTL).
  - `AuthService.Login`: after the password verifies, a SUSPENDED or BANNED user gets `PermissionDenied` with one generic fixed message `tài khoản bị khóa` for both statuses (decision A16: no suspended/banned distinction, no end time, no reason code, no note, no error details) and no session is created. A wrong password still returns the generic invalid-credentials error for every status (no status oracle). A SUSPENDED user whose `status_until` is at or before now is lifted in the same call: status becomes `active`, `UserStatusChanged(ACTIVE, previous SUSPENDED, reason_code "suspension_expired")` is enqueued in the same transaction, and the login proceeds. The lift is a conditional update (`status='suspended' AND status_until <= now`); when it updates 0 rows (another login lifted it, or an admin changed the status between the read and the lift) `Login` re-reads the user and runs the status gate again on the fresh row, so a ban that wins the race refuses the login. The lift is a system action and is not audited. An expired suspension is lifted only here (decision A15): a user who never logs in again stays `suspended`, their listings stay HELD and `ListUsers` with filter SUSPENDED still shows them; a lift sweeper is a follow-up.
  - `AuthService.RefreshSession`: after the token resolves to a session and the user loads, a stored SUSPENDED or BANNED status returns `PermissionDenied` and the session is revoked (with `SessionRevoked`). Unknown tokens still return the generic `Unauthenticated` first. Lifting by expiry happens only at login.
  - Session creation re-checks the status: `CreateSession` runs in one transaction that first reads the user's `status` and `status_until` with `SELECT ... FOR SHARE` and refuses with `ErrAccountLocked` (no session row) when the user is SUSPENDED or BANNED; `issue()` derives the token scopes from the status read in that transaction. `SetStatus` holds `FOR UPDATE` on the same row, so a ban either commits first (the insert sees `banned`) or waits for the insert and then revokes the new session. No session can survive a ban that committed before it.
  - `authz`: `ScopesForUser(roles, status)` returns `ScopesForRoles(roles)` minus `listing.write` for RESTRICTED; used by `issue()` and `RefreshSession`. A restricted user keeps every other scope (buy, chat, engagement).
  - Config: `UPSTREAM_AUDIT_ADDR` (default empty) and `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2), new `internal/upstream/audit.go` mirroring `team-domain/internal/upstream/audit.go`; `.env.example` and README updated.
  - Not changed: JWKS, access TTL, `Register` (new users start `active`), password reset, `SessionService`, `SessionRevoked` schema.
- **team-gateway**
  - `internal/edge/accountadmin.go`: `AccountAdminForwarder` (`ListUsers` via `callRead`, `SetUserStatus` via `callWrite`, never retried), registered in `server.go`; `AccountAdminService` added to the reflected service list.
  - `policy.go` `adminProcedures`: `ListUsers` and `SetUserStatus` need scope `admin` (anonymous 401, no scope 403 before forwarding).
  - `policy_edge_test.go` and `unrouted_test.go` updated; `ScreenListing` stays unrouted (other slice).
- Platform-e2e support: this slice adds an admin user-status helper under `platform-e2e/tests/e2e/support/` (owned by the e2e track).

## NFR

- `SetUserStatus` p95 < 500 ms for a user with up to 20 active sessions at identity (non-gating target; audit round trip plus one transaction).
- Lockout < 1 min end to end: the outbox relayer publishes within its poll interval, the gateway denylist consumer applies it, and open SSE streams end per existing `edge.sse-ends-on-session-revoke`.
- Availability: audit down, slow (> `UPSTREAM_CALL_TIMEOUT_SECONDS`) or unconfigured returns `Unavailable` and changes nothing; identity login and refresh do not depend on audit.
- Limits: `reason_code` 1 to 64 bytes; `note` at most 500 characters; `ListUsers` `query` at most 64 bytes, page size default 20 and max 100. Suspension `until` must be strictly after now; no maximum (a longer penalty is a ban).
- Login adds one indexed read of the user row it already loads; no extra round trip on the success path.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `users.status`, `status_reason` (reason code), `status_until`, `status_changed_at` | enforcement record about a person | restrict, suspend or ban an account; show the admin the current state | platform duty to act against violations (Decree 85/2021); DPO sign-off is a NEEDS_HUMAN of the initiative | while the account exists; audit copy per Q3 |
| admin `note` (free text, may contain PII) | personal, free text | context for the decision | same | not stored in `users`; kept only in the team-audit event metadata under Q3 retention |
| `UserStatusChanged` payload (user id, statuses, reason code, until, changed_at) | enforcement record | let team-domain hold and restore listings | same | Kafka retention of `identity.events`; carries no note, no username |
| `ListUsers` response (id, username, roles, status, reason, until, created_at) | personal | admin lookup | legitimate interest of the platform operator | not stored; response only; not logged |

The note, the password hash and refresh hashes MUST NOT appear in logs, spans, metrics labels or events. The login refusal message never reveals the reason code or note.

## Threat model

- **Assets:** user accounts and sessions, the enforcement record, the admin scope.
- **Abuse cases:**
  - A non-admin (or anonymous) calls `SetUserStatus` or `ListUsers` to lock out or enumerate users: edge 401/403 and identity re-checks scope `admin` itself (defence in depth).
  - An admin locks themselves out or demotes another admin: refused with `FailedPrecondition`.
  - Ban racing a login: a ban that commits after the login's status gate but before the session insert would leave a fresh session valid for one access TTL (`JWT_TTL_SECONDS`, default 3600); the session insert re-reads the status under a row lock in its own transaction, so it is refused, and a session committed first is revoked by the ban.
  - Ban racing the expiry lift: the lift is conditional and a 0-row update re-runs the gate on a fresh read, so a miss is never treated as lifted.
  - A banned user keeps using a live access token: all sessions are revoked in the same transaction and the gateway denylist applies within one minute; the user cannot log in again (password right or not); refresh tokens are cleared.
  - Login probing for account state: a wrong password returns the generic error for every status; the status is only revealed after the correct password.
  - Enforcement without a trail: audit is written first and fail closed, so no status change exists without a `user.set_status` event.
  - Race between two logins lifting the same expired suspension: the lift is a conditional update, so exactly one `UserStatusChanged(ACTIVE)` is emitted.
  - Partial failure: status update, session revokes and outbox rows are one transaction; a failure rolls all of them back, so a user is never marked banned with live sessions.
  - Restricted user regaining `listing.write`: scopes are re-derived from the stored status at every issue and refresh; the session revoke forces a new token.
  - LIKE-wildcard injection in `ListUsers` query: `%`, `_` and `\` are escaped; prefix match only.
- **Authz rule:** `AccountAdminService` RPCs require a user principal holding scope `admin`; `SetUserStatus` additionally forbids the caller as target and any target holding role `admin`. The actor id in the audit event is the verified principal id, never a request field.
