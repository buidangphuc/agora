# frontend-session-continuity Specification

## Purpose
The frontend keeps users signed in by storing a refresh cookie, silently refreshing expired access tokens in middleware, and revoking the session on logout.

## Requirements

### Requirement: Login and register store a refresh cookie

After a successful `loginAction` or `registerAction`, the system SHALL store the returned refresh token in
the cookie `agora_refresh` with `httpOnly`, `sameSite: lax`, `path: /`, `maxAge` 2592000 seconds and `secure` set
exactly when the `session` cookie is secure.

#### Scenario: S1 Login sets both cookies
- **WHEN** `AuthService.Login` returns a token and a non-empty `refreshToken`
- **THEN** the system SHALL set `session` (maxAge 3600) and `agora_refresh` (httpOnly, lax, path `/`, maxAge 2592000, `secure` equal to the `secure` flag of `session`) before redirecting

#### Scenario: S2 Register sets both cookies
- **WHEN** `AuthService.Register` returns a token and a non-empty `refreshToken`
- **THEN** the system SHALL set `session` and `agora_refresh` with the same attributes as S1

#### Scenario: S3 Sessions disabled (boundary)
- **WHEN** login succeeds with an empty `refreshToken`
- **THEN** the system SHALL set only `session`, set no `agora_refresh`, and still redirect
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit; not producible end to end — Clarifications round 2)

#### Scenario: S4 Failed login sets nothing (error)
- **WHEN** `AuthService.Login` fails with Unauthenticated
- **THEN** the system SHALL set no cookie
- **AND** the system SHALL return `ok: false` with the existing error message

### Requirement: Middleware silently refreshes an unusable access token

An access token is usable per `isUsableToken()` in `lib/gateway/token-expiry.ts`: `exp` minus a 5 s skew is in the
future, decoded without signature verification. When the `session` cookie is missing or not usable AND `agora_refresh`
is present, the middleware (Node runtime, connect-node client) SHALL call `AuthService.RefreshSession` once.

#### Scenario: S5 Expired access, valid refresh
- **WHEN** a request has an expired `session` cookie and a valid `agora_refresh`
- **THEN** the middleware SHALL call `RefreshSession` exactly once
- **AND** the system SHALL set a new `session` cookie and a rotated `agora_refresh` cookie on the response (same attributes as S1)

#### Scenario: S6 Missing access, valid refresh
- **WHEN** a request has no `session` cookie and a valid `agora_refresh`
- **THEN** the system SHALL behave as S5

#### Scenario: S7 Usable access token (boundary)
- **WHEN** a request carries a usable `session` cookie, with or without `agora_refresh`
- **THEN** the system SHALL make zero `RefreshSession` calls

#### Scenario: S8 No refresh cookie
- **WHEN** the `session` cookie is expired or missing and no `agora_refresh` exists
- **THEN** the system SHALL make no `RefreshSession` call
- **AND** the system SHALL keep today's behaviour (stale `session` cleared, anonymous render)

#### Scenario: S9 Refresh rejected (error)
- **WHEN** `RefreshSession` fails with Unauthenticated (expired, revoked, reused)
- **THEN** the system SHALL delete both `session` and `agora_refresh` in the browser response
- **AND** the system SHALL delete both cookies in the request the render sees
- **AND** the page SHALL render anonymous

#### Scenario: S10 Gateway unavailable (error)
- **WHEN** `RefreshSession` times out after 3 s or fails with any error other than Unauthenticated
- **THEN** the system SHALL render anonymous
- **AND** the system SHALL leave `agora_refresh` and the stale `session` cookie exactly as received
- **AND** the system SHALL NOT retry

#### Scenario: S11 Empty refresh response (error)
- **WHEN** `RefreshSession` succeeds but returns an empty access token
- **THEN** the system SHALL treat it as S9
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit; not producible end to end — Clarifications round 2)

#### Scenario: S12 Refresh call carries no bearer (permission)
- **WHEN** the middleware calls `RefreshSession`
- **THEN** the request SHALL carry no `Authorization` header, so the gateway never sees the stale access token
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit; not producible end to end — Clarifications round 2)

#### Scenario: S19 Grace replay keeps the refresh cookie
- **WHEN** `RefreshSession` succeeds with a non-empty access token and an empty `refresh_token` (grace replay)
- **THEN** the system SHALL set only the `session` cookie
- **AND** the system SHALL keep the existing `agora_refresh` cookie unchanged

#### Scenario: S20 Access token expiring exactly now (boundary)
- **WHEN** a request carries a `session` cookie whose `exp` equals the current time (or is within the 5 s skew) and a valid `agora_refresh`
- **THEN** the system SHALL treat the token as not usable and call `RefreshSession` exactly once

### Requirement: Same-request signed-in render

After a successful refresh, the middleware SHALL make the new access token visible to the same request's render.

#### Scenario: S21 Render is signed in on the refreshing request
- **WHEN** the middleware refreshes successfully (S5, S6 or S19)
- **THEN** the page rendered for that same request SHALL be signed in as the shopper of the new access token, with no second navigation

#### Scenario: S18 Parallel requests share one refresh cookie
- **WHEN** two requests with the same `agora_refresh` and no usable `session` arrive in parallel
- **THEN** both SHALL render signed in
- **AND** the session SHALL NOT be revoked

### Requirement: Middleware exposes the request path

The middleware SHALL set a request header `x-pathname` on every request, for use by slice `frontend-login-return`.

#### Scenario: S23 x-pathname carries path and query
- **WHEN** a request for `/cart?coupon=A1` passes through the middleware, signed in, anonymous or refreshed
- **THEN** the request seen by the render SHALL carry `x-pathname: /cart?coupon=A1`
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit; not producible end to end — Clarifications round 2)

#### Scenario: S26 Client-supplied x-pathname is not trusted (error)
- **WHEN** an incoming request already carries an `x-pathname` header with a value that differs from its URL
- **THEN** the middleware SHALL overwrite it with the real path and query of the request
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit; not producible end to end — Clarifications round 2)

### Requirement: Logout revokes the server-side session

`logoutAction` SHALL call `SessionService.RevokeSession` for the current session, then clear both cookies.

#### Scenario: S13 Logout revokes and clears
- **WHEN** a signed-in shopper logs out and the usable access token carries `sid` X
- **THEN** the system SHALL call `RevokeSession({ sessionId: X })` with the shopper's bearer
- **AND** the system SHALL delete `session` and `agora_refresh` and redirect to `/`

#### Scenario: S14 Revoke fails (error)
- **WHEN** `RevokeSession` fails or times out after 3 s
- **THEN** the system SHALL still delete both cookies and redirect to `/`

#### Scenario: S15 Logout with only a refresh cookie
- **WHEN** the access token is missing or not usable and `agora_refresh` exists
- **THEN** the system SHALL call `RefreshSession` (no bearer) to obtain a fresh access token and read its `sid`
- **AND** the system SHALL call `RevokeSession` with that `sid` and the fresh bearer
- **AND** the system SHALL delete both cookies and redirect to `/`

#### Scenario: S24 Logout refresh-only path fails (error)
- **WHEN** logout takes the S15 path and `RefreshSession` fails or returns no `sid`, or the following `RevokeSession` fails
- **THEN** the system SHALL delete both cookies and redirect to `/`

#### Scenario: S25 Logout with no cookies (boundary)
- **WHEN** neither a usable access token nor `agora_refresh` exists
- **THEN** the system SHALL make no gateway call, delete both cookies and redirect to `/`
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit; not producible end to end — Clarifications round 2)

#### Scenario: S16 Revoked session cannot refresh
- **WHEN** a shopper logs out
- **AND** a later request presents the old `agora_refresh` value
- **THEN** `RefreshSession` SHALL answer Unauthenticated
- **AND** the render SHALL be anonymous (end to end with identity-session-refresh)

#### Scenario: S17 Logout only affects own session (permission)
- **WHEN** logout runs
- **THEN** the system SHALL pass only the `sid` from the caller's own access token (or from the refresh the caller's own cookie produced) and no user-supplied session id
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit; not producible end to end — Clarifications round 2)
