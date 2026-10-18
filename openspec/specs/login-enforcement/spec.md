# login-enforcement Specification

## Purpose
team-identity refuses login and session refresh for suspended and banned users, lifts expired suspensions, and limits restricted users to losing listing.write.

## Requirements

### Requirement: Suspended and banned users cannot log in

`AuthService.Login` SHALL, after the password verifies, refuse a user whose stored status is `suspended` (with `status_until` after now) or `banned` with `PermissionDenied` and the one generic message `tài khoản bị khóa` for both statuses, with no error details, SHALL create no session and SHALL record the attempt in login history as a failure. The session insert SHALL run in one transaction that re-reads the user's status under a row lock (`FOR SHARE`) and SHALL refuse the same way, inserting no row, when that status is `suspended` or `banned`; the token scopes SHALL be derived from the status read in that transaction. A wrong password SHALL return the same invalid-credentials error for every status.

#### Scenario: Login enforcement: Banned user is refused after a correct password
- **WHEN** a banned user logs in with the correct password
- **THEN** the call returns `PermissionDenied` with message `tài khoản bị khóa`, no session row is created and login history holds one failed attempt
- **VERIFIED BY**: team-identity `internal/service` `TestLoginBannedPermissionDenied`

#### Scenario: Login enforcement: Suspended user is refused until the end time
- **WHEN** a user suspended until now + 1 hour logs in with the correct password
- **THEN** the call returns `PermissionDenied` with message `tài khoản bị khóa`, no session row is created and the stored status is still `suspended`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginSuspendedPermissionDenied`

#### Scenario: Login enforcement: Wrong password reveals no status
- **WHEN** a banned user, a suspended user and an active user each log in with a wrong password
- **THEN** all three calls return the identical invalid-credentials error with the same code and message
- **VERIFIED BY**: team-identity `internal/service` `TestLoginWrongPasswordNoStatusOracle`

#### Scenario: Login enforcement: Refusal does not leak the reason or note
- **WHEN** a banned user with reason code `scam` and a note, and a suspended user with reason code `scam` and an `until`, are each refused at login
- **THEN** both errors have the identical code `PermissionDenied`, the identical message `tài khoản bị khóa` and no details, and neither contains `scam`, the note or the `until` time
- **VERIFIED BY**: team-identity `internal/handler` `TestLoginRefusalLeaksNothing`

#### Scenario: Login enforcement: Ban committed during login leaves no live session
- **WHEN** an admin's ban of a user commits after that user's Login passed the status gate and before the session insert, and in a second case the session insert commits before the ban
- **THEN** in the first case the session insert is refused, Login returns `PermissionDenied` with message `tài khoản bị khóa` and the user has 0 session rows; in the second case the ban returns `sessions_revoked` = 1 and the new session is revoked with 1 `SessionRevoked` row
- **VERIFIED BY**: team-identity `internal/repository` `TestCreateSessionRechecksStatusUnderLockPG` and team-identity `internal/service` `TestLoginBanRaceNoLiveSession`

#### Scenario: Login enforcement: Restricted and active users log in
- **WHEN** a restricted user and an active user log in with the correct password
- **THEN** both calls succeed and each creates one session
- **VERIFIED BY**: team-identity `internal/service` `TestLoginRestrictedAndActiveSucceed`

### Requirement: An expired suspension is lifted at login

When a user whose status is `suspended` and whose `status_until` is at or before now logs in with the correct password, `Login` SHALL, in one transaction, set status `active` and clear reason and until with a conditional update (`status = 'suspended' AND status_until <= now`), enqueue exactly one `UserStatusChanged` with status `active`, previous status `suspended` and reason code `suspension_expired`, and then issue the session normally. When the conditional update changes 0 rows, `Login` SHALL NOT treat the suspension as lifted: it SHALL re-read the user and run the status gate again on the fresh status (proceed for `active` or `restricted`, refuse for `suspended` or `banned`). The lift SHALL NOT write a team-audit event. A suspension SHALL be lifted only at login.

#### Scenario: Login enforcement: Expired suspension is lifted
- **WHEN** a user suspended until 1 second ago logs in with the correct password
- **THEN** the login succeeds, the user has status `active`, and the outbox holds exactly 1 `UserStatusChanged` with previous status `suspended` and reason code `suspension_expired`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginLiftsExpiredSuspension`

#### Scenario: Login enforcement: Suspension ending exactly now is lifted
- **WHEN** a user whose `status_until` equals the service clock logs in with the correct password
- **THEN** the login succeeds and the status becomes `active`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginSuspensionBoundaryIsLifted`

#### Scenario: Login enforcement: Concurrent logins lift once
- **WHEN** two logins with the correct password for the same expired suspension run concurrently
- **THEN** both succeed (the second sees 0 updated rows, re-reads status `active` and proceeds) and exactly 1 `UserStatusChanged(ACTIVE)` is in the outbox
- **VERIFIED BY**: team-identity `internal/repository` `TestLiftExpiredSuspensionOnceUnderConcurrencyPG`

#### Scenario: Login enforcement: Ban racing the lift refuses the login
- **WHEN** a user's suspension has expired and an admin bans the user after Login read the expired suspension and before the conditional lift runs
- **THEN** the lift updates 0 rows, Login returns `PermissionDenied` with message `tài khoản bị khóa`, no session is created, the status is still `banned` and the outbox holds no `UserStatusChanged` with reason code `suspension_expired`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginLiftMissRereadsStatus`

#### Scenario: Login enforcement: Failed lift fails the login closed
- **WHEN** the lift transaction fails with a database error during a login by a user with an expired suspension
- **THEN** the call returns `Unavailable`, no session is created and the status is still `suspended`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginLiftFailureFailsClosed`

#### Scenario: Login enforcement: Lift writes no audit event
- **WHEN** an expired suspension is lifted at login
- **THEN** the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/service` `TestLoginLiftNotAudited`

#### Scenario: Login enforcement: A banned user is never lifted by time
- **WHEN** a banned user with `status_until` in the past logs in with the correct password
- **THEN** the call returns `PermissionDenied` and the status is still `banned`
- **VERIFIED BY**: team-identity `internal/service` `TestLoginBannedNeverLifted`

### Requirement: RefreshSession refuses suspended and banned users

`AuthService.RefreshSession` SHALL, after the token resolves to a session and the owning user loads, return `PermissionDenied` with the message `tài khoản bị khóa` when the user's stored status is `suspended` or `banned`, SHALL revoke that session with a `SessionRevoked` outbox row in the same transaction, and SHALL NOT lift a suspension. An unknown, expired or reused token SHALL still return the generic `Unauthenticated` before any status check.

#### Scenario: Login enforcement: Surviving session of a banned user is refused and revoked
- **WHEN** a user's status is set to `banned` directly in the store without session revokes and the user presents a valid refresh token
- **THEN** the call returns `PermissionDenied`, the session is revoked and 1 `SessionRevoked` row is enqueued
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshBannedPermissionDeniedRevokes`

#### Scenario: Login enforcement: Expired suspension is not lifted by refresh
- **WHEN** a user whose suspension ended 1 minute ago presents a valid refresh token
- **THEN** the call returns `PermissionDenied` and the stored status is still `suspended`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshSuspendedNotLifted`

#### Scenario: Login enforcement: Unknown token is still generic
- **WHEN** an unknown refresh token is presented
- **THEN** the call returns `Unauthenticated` with the generic message, not `PermissionDenied`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshUnknownTokenBeforeStatusCheck`

#### Scenario: Login enforcement: Restricted user refreshes without listing.write
- **WHEN** a restricted user presents a valid refresh token
- **THEN** the call succeeds and the new access token's scopes contain no `listing.write`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshRestrictedDropsListingWrite`

### Requirement: Restricted users lose listing.write only

Access tokens issued by `Login` or `RefreshSession` for a user with status `restricted` SHALL carry the scopes of the user's roles minus `listing.write`. Every other scope of the roles SHALL be kept. Tokens for `active` users SHALL be unchanged. An `admin` user is never restricted.

#### Scenario: Login enforcement: Restricted seller token lacks listing.write
- **WHEN** a restricted seller logs in
- **THEN** the access token scopes equal the seller scopes without `listing.write` and still contain `listing.read`, `engagement:write` and `search:write`
- **VERIFIED BY**: team-identity `internal/authz` `TestScopesForUserRestricted` and team-identity `internal/service` `TestLoginRestrictedSellerNoListingWrite`

#### Scenario: Login enforcement: Active user scopes unchanged
- **WHEN** an active seller and an active admin log in
- **THEN** their scopes equal `ScopesForRoles` of their roles
- **VERIFIED BY**: team-identity `internal/authz` `TestScopesForUserActiveUnchanged`

#### Scenario: Login enforcement: Reinstated user regains listing.write at next refresh
- **WHEN** a restricted seller is set to `active` and then presents a valid refresh token
- **THEN** the new access token contains `listing.write`
- **VERIFIED BY**: team-identity `internal/service` `TestRefreshAfterReinstatementRestoresScope`

#### Scenario: Login enforcement: Unknown status gets no extra scopes
- **WHEN** `ScopesForUser` is called with an unrecognised status string
- **THEN** it returns the scopes of the roles minus `listing.write` (fail closed)
- **VERIFIED BY**: team-identity `internal/authz` `TestScopesForUserUnknownStatusFailsClosed`
