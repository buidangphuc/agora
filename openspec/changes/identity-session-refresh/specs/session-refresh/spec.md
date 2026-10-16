## ADDED Requirements

### Requirement: Login and Register issue an opaque refresh token

`AuthService.Login` and `AuthService.Register` SHALL, when sessions are enabled, return in `AuthResult` a `refresh_token` of 32 random bytes encoded base64url (43 characters) and `refresh_expires_at` equal to issue time plus 30 days. The service SHALL store only the SHA-256 hash of the token on the session row.

#### Scenario: Login returns a refresh token
- **WHEN** a user logs in with valid credentials
- **THEN** `AuthResult.refresh_token` is a 43-character base64url string and `refresh_expires_at` is 30 days (+/- 5 s) after now
- **VERIFIED BY**: team-identity `internal/service` `TestLoginIssuesRefreshToken`

#### Scenario: Register returns a refresh token
- **WHEN** a new buyer registers
- **THEN** `AuthResult.refresh_token` is non-empty and its session row holds a hash that differs from the token
- **VERIFIED BY**: team-identity `internal/service` `TestRegisterIssuesRefreshToken`

#### Scenario: Only the hash is persisted
- **WHEN** a refresh token is issued and the session row is read back
- **THEN** `sessions.refresh_hash` equals the hex SHA-256 of the token and the raw token appears in no column of the row
- **VERIFIED BY**: team-identity `internal/repository` `TestRefreshHashOnlyStored`

#### Scenario: Token and hash never reach logs, spans or outbox
- **WHEN** Login, a rotation, a grace replay and a reuse revocation run with log, span and outbox capture enabled
- **THEN** zero captured log lines, span attributes and outbox payloads contain the raw token or its hash
- **VERIFIED BY**: team-identity `internal/handler` `TestRefreshSecretsNeverLogged`

#### Scenario: Sessions disabled yields no refresh token
- **WHEN** the service runs without a session repository and a user logs in
- **THEN** `refresh_token` is empty and `refresh_expires_at` is unset
- **VERIFIED BY**: team-identity `internal/service` `TestLoginWithoutSessionsHasNoRefresh`

#### Scenario: Failed login issues nothing
- **WHEN** a login fails with a wrong password
- **THEN** the call returns `Unauthenticated` and no session row is created
- **VERIFIED BY**: team-identity `internal/service` `TestFailedLoginCreatesNoSession`

### Requirement: RefreshSession rotates the token with a sliding expiry

`AuthService.RefreshSession` SHALL accept a live refresh token and return a new access token and a new refresh token for the same session id (`sid`). It SHALL set `refresh_expires_at` to now plus 30 days, update `last_seen`, move the presented hash to `prev_refresh_hash`, and set `rotated_at`. Scopes SHALL be derived from the user's current roles.

#### Scenario: Successful rotation
- **WHEN** a client presents the current valid refresh token R1
- **THEN** the response holds an access token whose `sid` equals the original session id, a refresh token R2 different from R1, and `refresh_expires_at` 30 days (+/- 5 s) after now
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshRotatesAndKeepsSid`

#### Scenario: Expiry slides on use
- **WHEN** a session whose `refresh_expires_at` is 1 day away is refreshed
- **THEN** the new `refresh_expires_at` is 30 days after now
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshExtendsExpiry`

#### Scenario: The rotated token is the only live one
- **WHEN** R1 was rotated into R2 more than 10 s ago and the client presents R2
- **THEN** the call succeeds and returns R3
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshChain`

#### Scenario: Scopes follow current roles
- **WHEN** a user's roles changed between login and refresh
- **THEN** the new access token carries the scopes of the current roles
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshRederivesScopes`

#### Scenario: A token resolves only to its owning session
- **WHEN** sessions S1 (user A) and S2 (user B) both exist and S2's refresh token is presented
- **THEN** the response carries user B's principal and S2's `sid`, and S1's row (hashes, `refresh_expires_at`, `last_seen`, `rotated_at`) is unchanged
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshBindsToOwningSession`

#### Scenario: Missing or oversized token is Unauthenticated
- **WHEN** `refresh_token` is empty, or is 257 bytes
- **THEN** the call returns `Unauthenticated` with the same message as an unknown token (never `InvalidArgument`) and no session row changes
- **VERIFIED BY**: team-identity `internal/handler` `TestRefreshSessionRejectsBadInput`

#### Scenario: Maximum-length token is accepted for lookup
- **WHEN** `refresh_token` is exactly 256 bytes and matches no session
- **THEN** the call returns `Unauthenticated` through the normal lookup
- **VERIFIED BY**: team-identity `internal/handler` `TestRefreshSessionMaxLengthBoundary`

#### Scenario: Refresh is callable without a Principal
- **WHEN** `RefreshSession` is called with no principal metadata
- **THEN** the call is not rejected for missing authentication and the authz coverage test classifies it as public
- **VERIFIED BY**: team-identity `internal/authz` `TestRefreshSessionIsPublic`

#### Scenario: Identity database down is not a logout
- **WHEN** the database is unreachable during `RefreshSession` with a valid token
- **THEN** the call returns `Unavailable`, and after the database recovers the same token is still accepted and the session is not revoked
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshDBDownIsUnavailableNotRevoked`

### Requirement: Invalid refresh tokens are rejected as Unauthenticated

`RefreshSession` SHALL return `Unauthenticated` with the same message for an empty, oversized (over 256 bytes), unknown, expired, revoked, reused or stale token, a token past the absolute lifetime, or a token whose user no longer exists, and SHALL NOT change any session row for an empty, oversized, unknown or expired token. WHEN `now` is later than the session's `created_at` plus 90 days (absolute lifetime, independent of the sliding expiry), THE SYSTEM SHALL return the same `Unauthenticated` and revoke the session. A grace replay SHALL also load the current user: a deleted user gets `Unauthenticated` and the session is revoked; scopes are re-derived.

#### Scenario: Unknown token
- **WHEN** a random 43-character token is presented
- **THEN** the call returns `Unauthenticated`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshUnknownToken`

#### Scenario: Expired token
- **WHEN** the session's `refresh_expires_at` is in the past
- **THEN** the call returns `Unauthenticated` and the session is not revoked
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshExpiredToken`

#### Scenario: Revoked session
- **WHEN** the session was revoked through `SessionService.RevokeSession` and its refresh token is presented
- **THEN** the call returns `Unauthenticated` and publishes no further event
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshRevokedSession`

#### Scenario: Owning user deleted
- **WHEN** the user who owns a live session is deleted and the session's refresh token is presented
- **THEN** the call returns `Unauthenticated`, the session is revoked, and exactly one `SessionRevoked` outbox row is written
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshDeletedUserRevokes`

#### Scenario: Owning user deleted, grace replay
- **WHEN** R1 (rotated into R2) is replayed inside the grace window after the owning user was deleted
- **THEN** the call returns `Unauthenticated`, the session is revoked, and exactly one `SessionRevoked` outbox row is written
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshGraceDeletedUserRevokes`

#### Scenario: Grace replay re-derives scopes
- **WHEN** a user's roles changed after rotation and R1 is replayed inside the grace window
- **THEN** the access token carries the scopes of the current roles
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshGraceRederivesScopes`

#### Scenario: Absolute lifetime exceeded
- **WHEN** the session's `created_at` is 90 days plus 1 s ago, `refresh_expires_at` is still in the future, and its current token is presented
- **THEN** the call returns `Unauthenticated` with the generic message, the session is revoked, and exactly one `SessionRevoked` outbox row is written
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshAbsoluteLifetimeRevokes`

#### Scenario: Absolute lifetime boundary
- **WHEN** a session's `created_at` is exactly 90 days ago and its current token is presented
- **THEN** the call succeeds and rotates
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshAbsoluteLifetimeBoundary`

#### Scenario: Failure messages do not leak the cause
- **WHEN** an unknown, expired, revoked, deleted-user, empty and 257-byte token are each presented
- **THEN** the six responses carry an identical status code and message
- **VERIFIED BY**: team-identity `internal/handler` `TestRefreshErrorsAreIndistinguishable`

### Requirement: Reuse of a rotated token revokes the session

WHEN the immediately previous refresh token is presented more than 10 s after rotation, THE SYSTEM SHALL revoke the whole session, clear its refresh hashes and publish `SessionRevoked` in the same database transaction. Within 10 s (inclusive) of rotation, `RefreshSession` SHALL return a fresh access token for the same `sid` with an empty `refresh_token`, without rotating. A token older than one rotation SHALL get `Unauthenticated` without revoking the session. Revoking SHALL clear both hashes, so every token of a revoked session, including a prev-hash replay inside the grace window, SHALL get `Unauthenticated`.

#### Scenario: Reuse after the grace window revokes
- **WHEN** R1 was rotated into R2 and R1 is presented 11 s later
- **THEN** the call returns `Unauthenticated`, R2 is rejected afterwards with `Unauthenticated`, `ListSessions` for the user no longer lists the session as active, and the row's `refresh_hash` and `prev_refresh_hash` are NULL
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshReuseRevokesSession`

#### Scenario: SessionRevoked is published on reuse
- **WHEN** reuse is detected
- **THEN** exactly one outbox row of type `platform.identity.v1.SessionRevoked` is written with the session id and user id, and `expires_at` is now plus the access-token TTL
- **VERIFIED BY**: team-identity `internal/repository` `TestReuseWritesSessionRevokedOutbox` (Postgres)

#### Scenario: Revoke and event are atomic
- **WHEN** writing the outbox row fails during reuse handling
- **THEN** the session stays unrevoked and the call returns `Internal`, not `Unauthenticated`
- **VERIFIED BY**: team-identity `internal/repository` `TestReuseOutboxFailureRollsBack`

#### Scenario: Grace window boundary
- **WHEN** R1 is re-presented exactly 10 s after rotation, and again at 10 s plus 1 ms
- **THEN** the first call returns an access token with the original `sid` and an empty `refresh_token`, and the second returns `Unauthenticated` and revokes the session
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshGraceBoundary`

#### Scenario: Grace replay returns an access token only
- **WHEN** R1 (rotated into R2) is replayed inside the grace window
- **THEN** the response has a valid access token for the same `sid`, an empty `refresh_token`, and the row still holds R2's hash and its `refresh_expires_at` unchanged
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshGraceReturnsAccessOnly`

#### Scenario: Parallel tabs do not trip reuse detection
- **WHEN** two requests present R1 concurrently
- **THEN** exactly one call returns a new refresh token R2, the other returns an access token for the same `sid` with an empty `refresh_token`, and the session stays active
- **VERIFIED BY**: team-identity `internal/repository` `TestRefreshConcurrentSameToken` (Postgres, row lock)

#### Scenario: Token older than one rotation is rejected without revoke
- **WHEN** R1 was rotated into R2, then R2 into R3, and R1 is presented
- **THEN** the call returns `Unauthenticated` and the session stays active with R3 still accepted
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshStaleTokenNoRevoke`

#### Scenario: Prev-token replay within grace after logout
- **WHEN** R1 was rotated into R2, the user calls `SessionService.RevokeSession` and R1 is replayed 5 s after rotation
- **THEN** the call returns `Unauthenticated` (same message as unknown), no access token is issued and no further outbox row is written
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshPrevTokenAfterLogoutUnauthenticated`

#### Scenario: Lost response retried after the grace window counts as reuse
- **WHEN** a rotation response is lost and the client retries R1 11 s later
- **THEN** the call returns `Unauthenticated` and the session is revoked
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshLostResponseRetryIsReuse`

### Requirement: Gateway forwards RefreshSession as a public, non-retried RPC

The gateway SHALL expose `platform.identity.v1.AuthService/RefreshSession` without requiring a Principal and SHALL forward it to team-identity exactly once (no retry). No bearer-skip exception exists because callers send no `Authorization` header.

#### Scenario: Anonymous caller reaches identity
- **WHEN** a request with no `Authorization` header calls `RefreshSession` through the gateway
- **THEN** the request is forwarded to identity and the identity response is returned unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestRefreshSessionForwardedPublic`

#### Scenario: Upstream Unavailable is not retried
- **WHEN** identity answers `Unavailable` to a forwarded `RefreshSession`
- **THEN** identity receives exactly 1 call and the caller gets `Unavailable`
- **VERIFIED BY**: team-gateway `internal/edge` `TestRefreshSessionNotRetried`

#### Scenario: Unauthenticated passes through
- **WHEN** identity answers `Unauthenticated`
- **THEN** the gateway returns Connect code `unauthenticated`
- **VERIFIED BY**: team-gateway `internal/edge` `TestRefreshSessionUnauthenticatedPassthrough`

#### Scenario: Anonymous rate-limit bucket applies
- **WHEN** one anonymous client IP sends 41 `RefreshSession` calls at once against the limiter configured as in `internal/edge/interceptors.go` (`RATE_LIMIT_BURST` N = 40, `RATE_LIMIT_RPS` 20/s refill, per-IP bucket)
- **THEN** calls 1 to 40 are forwarded and call 41 is answered `resource_exhausted` (`ResourceExhausted`) by the gateway; identity receives exactly 40 calls
- **VERIFIED BY**: team-gateway `internal/edge` `TestRefreshSessionAnonymousRateLimited`

#### Scenario: Procedure is covered by the unrouted guard
- **WHEN** the unrouted-procedure test enumerates identity `AuthService` procedures
- **THEN** `RefreshSession` is routed (not `Unimplemented`)
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardIncludesRefreshSession`

### Requirement: A revoked session can never be refreshed

WHILE a session is revoked, THE SYSTEM SHALL answer `Unauthenticated` to `RefreshSession` for any token of that session, including after the `SessionRevoked` denylist entry has expired. `RevokeSession` SHALL write the denylist `expires_at` as now plus the access-token TTL.

#### Scenario: Revocation outlives the denylist
- **WHEN** a session is revoked and its denylist TTL has elapsed
- **THEN** a refresh with the last issued token still returns `Unauthenticated`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshRevokedAfterDenylistExpiry`

#### Scenario: RevokeSession denylist expiry is now plus access TTL
- **WHEN** a user revokes a session via `SessionService.RevokeSession` with access TTL 3600 s
- **THEN** the `SessionRevoked` outbox row has `expires_at` equal to now + 3600 s (+/- 5 s) and the row's refresh hashes are NULL
- **VERIFIED BY**: team-identity `internal/service` `TestRevokeSessionDenylistExpiry`

#### Scenario: Logout then refresh, end to end
- **WHEN** a signed-in user logs out via `SessionService.RevokeSession` and then calls `RefreshSession` through the gateway with the old refresh token
- **THEN** the response is `unauthenticated` and no new access token is issued
- **VERIFIED BY**: e2e `identity-session-refresh/logout-then-refresh`

### Requirement: Migration is additive and reversible

Migration `0006_session_refresh` SHALL add the four columns `refresh_hash`, `prev_refresh_hash`, `refresh_expires_at`, `rotated_at` as nullable, a unique index on `refresh_hash` and an index on `prev_refresh_hash`, and SHALL NOT break sessions created before the migration.

#### Scenario: Legacy session without a refresh hash
- **WHEN** a session row created before the migration (null `refresh_hash`) is listed or revoked
- **THEN** `ListSessions` and `RevokeSession` behave as before and no refresh token matches it
- **VERIFIED BY**: team-identity `internal/repository` `TestLegacySessionUnaffectedByRefreshColumns` (Postgres)

#### Scenario: Down migration restores the old schema
- **WHEN** the down migration is applied
- **THEN** the four columns and both indexes no longer exist
- **VERIFIED BY**: team-identity `internal/repository` `TestMigration0006Down` (Postgres)

#### Scenario: Duplicate hash is refused
- **WHEN** two sessions are written with the same `refresh_hash`
- **THEN** the second insert fails with a unique violation
- **VERIFIED BY**: team-identity `internal/repository` `TestRefreshHashUnique` (Postgres)
