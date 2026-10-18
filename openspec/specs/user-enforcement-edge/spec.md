# user-enforcement-edge Specification

## Purpose
The gateway routes AccountAdminService behind the admin scope, and a ban ends the user's access within one minute.

## Requirements

### Requirement: Gateway routes AccountAdminService behind the admin scope

team-gateway SHALL register `AccountAdminService` and forward `ListUsers` once through `callRead` and `SetUserStatus` once through `callWrite` (never retried). Both procedures SHALL be in `adminProcedures` with scope `admin`, so the edge answers anonymous callers `unauthenticated` and callers without the scope `permission_denied` before forwarding.

#### Scenario: User enforcement edge: Admin reaches SetUserStatus
- **WHEN** an admin calls `SetUserStatus` through the gateway
- **THEN** the request is forwarded to team-identity once with the caller's principal and the response is returned unchanged
- **VERIFIED BY**: team-gateway `internal/edge` `TestAccountAdminSetUserStatusForwarded`

#### Scenario: User enforcement edge: Anonymous caller is rejected at the edge
- **WHEN** an anonymous caller calls `ListUsers` or `SetUserStatus`
- **THEN** each call returns `unauthenticated` and team-identity receives 0 requests
- **VERIFIED BY**: team-gateway `internal/edge` `TestAccountAdminAnonymousUnauthenticated`

#### Scenario: User enforcement edge: Seller without admin scope is denied
- **WHEN** a signed-in seller calls `ListUsers` or `SetUserStatus`
- **THEN** each call returns `permission_denied` and team-identity receives 0 requests
- **VERIFIED BY**: team-gateway `internal/edge` `TestAccountAdminNonAdminPermissionDenied`

#### Scenario: User enforcement edge: SetUserStatus is not retried
- **WHEN** team-identity answers `SetUserStatus` with `Unavailable`
- **THEN** the gateway returns `unavailable` after exactly 1 upstream call
- **VERIFIED BY**: team-gateway `internal/edge` `TestAccountAdminSetUserStatusNotRetried`

#### Scenario: User enforcement edge: Admin policy set is pinned
- **WHEN** the policy test compares `adminProcedures` with the expected set
- **THEN** the set additionally contains exactly `AccountAdminServiceListUsersProcedure` and `AccountAdminServiceSetUserStatusProcedure`
- **VERIFIED BY**: team-gateway `internal/edge` `TestAdminProceduresPinned`

#### Scenario: User enforcement edge: Procedures are covered by the unrouted guard
- **WHEN** the unrouted guard enumerates generated procedures
- **THEN** both `AccountAdminService` procedures are routed and listed, and no procedure of the service is unrouted
- **VERIFIED BY**: team-gateway `internal/edge` `TestUnroutedGuardAccountAdmin`

### Requirement: A ban ends the user's access within one minute

After `SetUserStatus` commits a `restricted`, `suspended` or `banned` status, the user's access tokens SHALL stop working at the gateway and their open SSE streams SHALL end within 60 seconds, through the existing `SessionRevoked` denylist path.

#### Scenario: User enforcement edge: Banned user's token is rejected within a minute
- **WHEN** a logged-in buyer is banned by an admin and the buyer then calls an authenticated RPC with the old access token
- **THEN** within 60 s the call returns `unauthenticated`
- **VERIFIED BY**: platform-e2e `features/admin/user_enforcement.feature` "Banned user loses access within a minute"

#### Scenario: User enforcement edge: Banned user's open stream ends
- **WHEN** a buyer holds an open notification SSE stream and an admin bans them
- **THEN** the stream ends within 60 s
- **VERIFIED BY**: platform-e2e `features/admin/user_enforcement.feature` "Banned user's open stream ends"
