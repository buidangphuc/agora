## ADDED Requirements

### Requirement: Refresh and revoke carry the browser's client context

`refreshSession()` and the revoke calls (`revokeOwnSession()`, `sessions.ts revokeSession()`) SHALL send
`X-Forwarded-For` and `User-Agent` derived by `clientContextFrom(headers)` of the inbound browser request, exactly as
login and register do. The IP SHALL be the entry `TRUSTED_PROXY_HOPS` from the right of the inbound
`X-Forwarded-For`, never the leftmost; invalid values SHALL be omitted. `listSessions` and `listLoginHistory` SHALL NOT
carry client context.

#### Scenario: Session client context: middleware refresh forwards IP and UA
- **WHEN** the middleware refreshes an expired access token for a request with `X-Forwarded-For: 203.0.113.9` (one trusted hop) and `User-Agent: AgoraTest/2`
- **THEN** the `RefreshSession` request to the gateway SHALL carry `X-Forwarded-For: 203.0.113.9` and `User-Agent: AgoraTest/2`
- **AND** it SHALL carry no `Authorization` header
- **VERIFIED BY**: `team-frontend/src/lib/gateway/refresh.test.ts` and `team-frontend/src/middleware.test.ts` (unit; the container-to-gateway headers are not observable from the browser)

#### Scenario: Session client context: logout refresh and revoke forward IP and UA
- **WHEN** `logoutAction` runs with an expired `session` cookie and a valid `agora_refresh`, from a request with `X-Forwarded-For: 203.0.113.9` and `User-Agent: AgoraTest/2`
- **THEN** both the `RefreshSession` call and the following `RevokeSession` call SHALL carry `X-Forwarded-For: 203.0.113.9` and `User-Agent: AgoraTest/2`
- **AND** the `RevokeSession` call SHALL carry the refreshed access token as bearer
- **VERIFIED BY**: `team-frontend/src/features/auth/actions.test.ts` (unit)

#### Scenario: Session client context: revoke another device forwards IP and UA
- **WHEN** a signed-in user revokes one of their sessions from the sessions page
- **THEN** the `RevokeSession` request SHALL carry the user's `X-Forwarded-For` and `User-Agent` and a bearer token
- **AND** the `ListSessions` request from the same page SHALL carry neither header
- **VERIFIED BY**: `team-frontend/src/lib/gateway/sessions.test.ts` (unit)

#### Scenario: Session client context: spoofed leftmost X-Forwarded-For is ignored (permission)
- **WHEN** a browser sends `X-Forwarded-For: 6.6.6.6` and the trusted proxy appends the real peer, giving `6.6.6.6, 198.51.100.4` with `TRUSTED_PROXY_HOPS=1`
- **THEN** the forwarded IP SHALL be `198.51.100.4`
- **AND** WHEN the chain has fewer entries than the hop count, or the selected entry is not an IP, THE SYSTEM SHALL forward no `X-Forwarded-For`
- **VERIFIED BY**: `team-frontend/src/lib/gateway/client-context.test.ts` (unit)

#### Scenario: Session client context: missing headers (boundary)
- **WHEN** the inbound request has no `User-Agent` and no `X-Forwarded-For`
- **THEN** the refresh and revoke calls SHALL still be made, with neither header set, and succeed as today
- **VERIFIED BY**: `team-frontend/src/lib/gateway/refresh.test.ts` (unit)

#### Scenario: Session client context: refresh failure behaviour unchanged (error)
- **WHEN** `RefreshSession` fails with Unauthenticated, or times out after 3 s, with client context attached
- **THEN** the system SHALL behave exactly as `frontend-session-continuity` S9 and S10 (clear both cookies; render anonymous and keep cookies)
- **VERIFIED BY**: `team-frontend/src/middleware.test.ts` (unit)
