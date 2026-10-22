# identity-session-rotation-client Specification

## Purpose
On refresh-token rotation, team-identity updates the session's ip, device and last_seen from the forwarded client context.

## Requirements

### Requirement: Rotation updates the session's ip, device and last_seen

When `AuthService.RefreshSession` rotates a refresh token (outcome `RotateRotated`), the system SHALL set the session's
`ip` to the forwarded `x-client-ip` (clipped to 64) and `device` to the forwarded `x-client-user-agent` (clipped to 256),
and `last_seen` to the rotation time, in the same statement that swaps the refresh hash. A forwarded value that is
empty SHALL leave the stored value unchanged. Any other outcome SHALL NOT change ip, device or last_seen. The values
are audit-only and never used for authorization.

#### Scenario: Session client context: rotation updates device and IP
- **GIVEN** a session created at login from `AgoraTest/1`
- **WHEN** the browser's `session` cookie expires and the next page load refreshes it from a browser whose `User-Agent` is `AgoraTest/2`
- **THEN** the "Active sessions" page SHALL list that session with device `AgoraTest/2` and no entry with `AgoraTest/1` for it
- **AND** its last-seen time SHALL be later than its created time

#### Scenario: Session client context: rotation records IP via repository
- **WHEN** `RefreshSession` rotates a session while ctx carries `ClientInfo{IP: "198.51.100.4", UserAgent: "AgoraTest/2"}`
- **THEN** the stored row SHALL have `ip = 198.51.100.4`, `device = AgoraTest/2` and `last_seen = now`
- **AND** `refresh_hash` SHALL be the new hash
- **VERIFIED BY**: `team-identity/internal/service/auth_refresh_test.go` and `team-identity/internal/repository/session_test.go` (unit and repository against Postgres and in-memory; the browser cannot set the IP in the e2e stack)

#### Scenario: Session client context: empty client keeps stored values (boundary)
- **WHEN** `RefreshSession` rotates a session while ctx carries no `ClientInfo` (or empty fields)
- **THEN** `ip` and `device` SHALL equal their pre-rotation values
- **AND** `last_seen` SHALL still advance
- **AND** a 400-byte UA SHALL be stored clipped to 256 bytes and an IP over 64 bytes clipped to 64
- **VERIFIED BY**: `team-identity/internal/service/auth_refresh_test.go` (unit)

#### Scenario: Session client context: grace replay does not update (boundary)
- **WHEN** a refresh token already rotated is replayed within the grace window with a different `ClientInfo`
- **THEN** the call SHALL succeed with no new refresh token
- **AND** `ip`, `device`, `last_seen` SHALL be unchanged
- **VERIFIED BY**: `team-identity/internal/service/auth_refresh_test.go` (unit; timing-dependent, not stable end to end)

#### Scenario: Session client context: rejected refresh updates nothing (error)
- **WHEN** `RefreshSession` is called with an unknown, expired, reused or revoked-session token and a `ClientInfo`
- **THEN** the call SHALL fail with the existing error
- **AND** no session row SHALL have its `ip`, `device` or `last_seen` changed
- **VERIFIED BY**: `team-identity/internal/service/auth_refresh_test.go` (unit)

#### Scenario: Session client context: refresh needs no principal and grants nothing (permission)
- **WHEN** `RefreshSession` is called without an `Authorization` header and with a forged `x-client-ip` metadata sent straight at identity
- **THEN** the call SHALL authenticate by the refresh token only
- **AND** the values SHALL NOT alter the returned scopes or token claims
- **VERIFIED BY**: `team-identity/internal/handler/auth_test.go` (unit; identity trusts only gateway-stamped metadata, ADR-0010)
