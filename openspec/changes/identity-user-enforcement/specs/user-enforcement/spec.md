## ADDED Requirements

### Requirement: Users carry an enforcement status

team-identity SHALL store on each user a `status` (`active`, `restricted`, `suspended`, `banned`), a `status_reason` (reason code), an optional `status_until` and `status_changed_at`. A user created by `Register` or by seeding SHALL be `active`; rows that exist before the migration SHALL read as `active`.

#### Scenario: User enforcement: New user is active
- **WHEN** a buyer registers
- **THEN** the stored user has status `active`, empty `status_reason` and no `status_until`
- **VERIFIED BY**: team-identity `internal/repository` `TestCreateUserDefaultsToActive`

#### Scenario: User enforcement: Existing rows read as active after migration
- **WHEN** migration `0007_user_status` runs on a database holding users created before it
- **THEN** every existing user has status `active` and the down migration drops the four columns and the index
- **VERIFIED BY**: team-identity `migrations` `TestMigration0007UserStatusUpDown`

#### Scenario: User enforcement: Unknown status value is refused by the schema
- **WHEN** an insert or update sets `users.status` to `frozen`
- **THEN** the database rejects it with a check-constraint violation and the row is unchanged
- **VERIFIED BY**: team-identity `internal/repository` `TestUserStatusCheckConstraint`

### Requirement: Admin sets a user's status, audit first

`AccountAdminService.SetUserStatus` SHALL, for a caller holding scope `admin`, validate the request, load the target user, write one team-audit event, and then in one database transaction update the user's status, revoke every active session of the user (for target status `restricted`, `suspended` or `banned`) and enqueue the outbox events. The audit event SHALL have `actor_id` equal to the caller's verified principal id, `action` `user.set_status`, `target_type` `user`, `target_id` the user id and metadata `status`, `reason_code`, `note` (when given) and `until` (RFC 3339 UTC, when given). The event SHALL be written as service principal `service-team-identity` holding `audit.write`. The response SHALL carry the updated `AdminUser` and `sessions_revoked`. `reason_code` SHALL be 1 to 64 bytes of lower_snake_case (`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`). Re-setting the same `restricted`, `suspended` or `banned` status SHALL revoke every active session again. Setting `active` SHALL revoke no session, SHALL store the new `reason_code` in `status_reason` and SHALL clear `status_until`.

#### Scenario: User enforcement: Ban writes one audit event and revokes sessions
- **WHEN** an admin bans a buyer who has 3 active sessions with reason code `scam`
- **THEN** the RPC succeeds with `sessions_revoked` = 3, the user has status `banned` and `status_reason` `scam`, and team-audit holds exactly 1 event with the admin id as `actor_id`, `action` `user.set_status`, `target_type` `user`, the user id as `target_id` and metadata `status` = `banned`
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusBanAuditsAndRevokes`

#### Scenario: User enforcement: Audit is written before the change
- **WHEN** an admin calls SetUserStatus and the audit write is recorded by a fake writer
- **THEN** the fake receives the audit call before the repository transaction is invoked and the event carries the target id from the stored user and the actor id from the principal, not from the request
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusAuditPrecedesChange`

#### Scenario: User enforcement: Audit down refuses the change
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin suspends a user
- **THEN** each case returns `Unavailable`, the user row and all sessions are unchanged, and no outbox row is written
- **VERIFIED BY**: team-identity `internal/handler` `TestSetUserStatusAuditDownUnavailableNoChange`

#### Scenario: User enforcement: No audit upstream configured
- **WHEN** an admin calls SetUserStatus while `UPSTREAM_AUDIT_ADDR` is unset
- **THEN** the RPC returns `Unavailable` and the user row is unchanged
- **VERIFIED BY**: team-identity `internal/upstream` `TestNewAuditClientEmptyAddrIsNil` and team-identity `internal/service` `TestSetUserStatusNilAuditUnavailable`

#### Scenario: User enforcement: Audit is called as the service principal
- **WHEN** the audit client writes an event
- **THEN** the outgoing metadata carries principal id `service-team-identity`, type `service` and scope `audit.write`, and no caller principal is forwarded
- **VERIFIED BY**: team-identity `internal/upstream` `TestAuditUsesServicePrincipal`

#### Scenario: User enforcement: Change failure after audit keeps the audit row
- **WHEN** the audit write succeeds and the transaction then fails with an internal error
- **THEN** the RPC returns `Internal`, the user row and sessions are unchanged, no outbox row exists, and the audit event already written is not retracted
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusTxFailureAfterAudit`

#### Scenario: User enforcement: Status change and revokes are atomic
- **WHEN** the outbox insert for the second `SessionRevoked` fails during a suspend of a user with 3 sessions
- **THEN** the user status is unchanged, all 3 sessions are still active and no outbox row exists
- **VERIFIED BY**: team-identity `internal/repository` `TestSetUserStatusRollsBackOnOutboxFailure`

#### Scenario: User enforcement: Restrict revokes sessions and keeps the user able to log in
- **WHEN** an admin restricts a seller with 2 active sessions
- **THEN** `sessions_revoked` is 2, status is `restricted`, and a subsequent Login with the correct password succeeds
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusRestrictRevokesSessions`

#### Scenario: User enforcement: Reinstating to active revokes nothing
- **WHEN** an admin sets a restricted user with 1 active session to `active` with reason code `appeal_accepted`, and in a second case sets a suspended user with an `until` to `active`
- **THEN** `sessions_revoked` is 0, the session stays active, and the user has status `active`, `status_reason` `appeal_accepted`, no `status_until` and `status_changed_at` updated
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusActiveRevokesNothing`

#### Scenario: User enforcement: Setting the same status updates reason and until
- **WHEN** an admin suspends an already suspended user who has 1 active session with a later `until`
- **THEN** the call succeeds with `sessions_revoked` = 1, `status_until` holds the new value, one audit event is written, 1 `SessionRevoked` row and one `UserStatusChanged` with `previous_status` `suspended` are enqueued
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusSuspendExtend`

#### Scenario: User enforcement: Unknown user is NotFound without audit
- **WHEN** an admin calls SetUserStatus with a user id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusNotFoundNoAudit`

#### Scenario: User enforcement: Admin cannot change their own status
- **WHEN** an admin calls SetUserStatus with their own user id
- **THEN** the RPC returns `FailedPrecondition`, the fake audit writer receives 0 calls and the user row is unchanged
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusSelfRefused`

#### Scenario: User enforcement: Admin cannot change another admin
- **WHEN** an admin calls SetUserStatus on a user holding role `admin`
- **THEN** the RPC returns `FailedPrecondition`, the fake audit writer receives 0 calls and the user row is unchanged
- **VERIFIED BY**: team-identity `internal/service` `TestSetUserStatusAdminTargetRefused`

#### Scenario: User enforcement: Invalid input is rejected before any audit
- **WHEN** an admin calls SetUserStatus with, in turn, status UNSPECIFIED, an empty `reason_code`, a `reason_code` of 65 bytes, the `reason_code` values `Scam`, `scam-x`, `1scam`, `scam_`, `_scam`, `scam__x` and `scam x` (not lower_snake_case), a `note` of 501 characters, an empty `user_id`, `SUSPENDED` without `until`, `SUSPENDED` with `until` equal to now, `SUSPENDED` with `until` in the past, and `BANNED` with `until` set
- **THEN** each call returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/handler` `TestSetUserStatusInvalidInput`

#### Scenario: User enforcement: Input boundaries are accepted
- **WHEN** an admin calls SetUserStatus with a 64-byte lower_snake_case `reason_code`, the reason codes `a` and `counterfeit_seller2`, a 500-character `note`, and `SUSPENDED` with `until` one second after now
- **THEN** the call succeeds and the audit event carries the full note
- **VERIFIED BY**: team-identity `internal/handler` `TestSetUserStatusInputBoundaries`

#### Scenario: User enforcement: Non-admin cannot set a status
- **WHEN** a caller holding the seller scopes, and in a second case a caller with no principal, calls SetUserStatus
- **THEN** the first call returns `PermissionDenied`, the second returns `Unauthenticated`, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/handler` `TestSetUserStatusRequiresAdminScope`

#### Scenario: User enforcement: Audit metadata never leaks into logs
- **WHEN** SetUserStatus runs with log and span capture enabled and a note containing a marker string
- **THEN** zero captured log lines and span attributes contain the note
- **VERIFIED BY**: team-identity `internal/handler` `TestSetUserStatusNoteNeverLogged`

### Requirement: Status changes publish SessionRevoked and UserStatusChanged

For target status `restricted`, `suspended` or `banned`, `SetUserStatus` SHALL enqueue in the same transaction one `SessionRevoked` outbox row per session it revokes (same event shape and `expires_at` = now + access TTL as `SessionService.RevokeSession`), followed by exactly one `UserStatusChanged` row with `user_id`, `status`, `previous_status`, `reason_code`, `until` and `changed_at`. For target `active` it SHALL enqueue only `UserStatusChanged`. All rows SHALL be published on topic `identity.events` with the user id as key and `UserStatusChanged` SHALL use envelope type `platform.identity.v1.UserStatusChanged`.

#### Scenario: User enforcement: One SessionRevoked per session then one UserStatusChanged
- **WHEN** an admin suspends a user with 3 active sessions
- **THEN** the outbox holds 3 `SessionRevoked` rows, each with a distinct session id and `expires_at` = now + access TTL (+/- 5 s), followed by 1 `UserStatusChanged` row with status `suspended`, previous status `active` and `until` set
- **VERIFIED BY**: team-identity `internal/repository` `TestSetUserStatusOutboxRows`

#### Scenario: User enforcement: Events keep per-user order
- **WHEN** the outbox relayer publishes the rows of one SetUserStatus
- **THEN** the `UserStatusChanged` is published after all `SessionRevoked` rows of that call and all rows use the user id as key
- **VERIFIED BY**: team-identity `internal/repository` `TestSetUserStatusOutboxOrderPG`

#### Scenario: User enforcement: Event carries no note and no username
- **WHEN** a `UserStatusChanged` envelope is decoded after a ban with a note
- **THEN** its fields are only the contract fields and the serialized payload does not contain the note text or the username
- **VERIFIED BY**: team-identity `internal/events` `TestUserStatusChangedPayloadMinimal`

#### Scenario: User enforcement: User without sessions still emits the status event
- **WHEN** an admin bans a user who has no active session
- **THEN** `sessions_revoked` is 0, no `SessionRevoked` row is written and exactly 1 `UserStatusChanged` row is written
- **VERIFIED BY**: team-identity `internal/repository` `TestSetUserStatusNoSessions`

#### Scenario: User enforcement: Already revoked sessions are not revoked twice
- **WHEN** an admin bans a user who has 1 active and 2 already revoked sessions
- **THEN** `sessions_revoked` is 1 and exactly 1 `SessionRevoked` row is written
- **VERIFIED BY**: team-identity `internal/repository` `TestSetUserStatusSkipsRevokedSessions`

#### Scenario: User enforcement: Refresh tokens of revoked sessions are dead
- **WHEN** a user is banned and the previously issued refresh token of one of their sessions is presented to RefreshSession
- **THEN** the call returns `Unauthenticated` (unknown token) and no session row changes
- **VERIFIED BY**: team-identity `internal/service` `TestBannedUserRefreshTokenDead`

### Requirement: Admin lists and finds users

`AccountAdminService.ListUsers` SHALL, for a caller holding scope `admin`, return users ordered by username ascending, filtered by `status` when given and by `query` as an exact user id or a username prefix. It SHALL page with `common.v1.PageRequest` (default 20, maximum 100, larger values clamped) and SHALL NOT return the password hash. It SHALL NOT write an audit event.

#### Scenario: User enforcement: Find by username prefix
- **WHEN** an admin lists with query `nguyen` and users `nguyen.a`, `nguyen.b` and `tran.c` exist
- **THEN** the response holds exactly `nguyen.a` and `nguyen.b` in that order
- **VERIFIED BY**: team-identity `internal/repository` `TestListUsersPrefix`

#### Scenario: User enforcement: Find by exact id
- **WHEN** an admin lists with query equal to a user's id
- **THEN** the response holds exactly that user
- **VERIFIED BY**: team-identity `internal/repository` `TestListUsersExactID`

#### Scenario: User enforcement: Filter by status
- **WHEN** an admin lists with status `SUSPENDED` and 2 of 10 users are suspended
- **THEN** the response holds exactly those 2 users with `status_reason` and `status_until` set
- **VERIFIED BY**: team-identity `internal/repository` `TestListUsersStatusFilter`

#### Scenario: User enforcement: Wildcards in the query are literal
- **WHEN** an admin lists with query `%` or `_` or `a\`
- **THEN** no user whose username does not start with that literal text is returned and the call does not fail
- **VERIFIED BY**: team-identity `internal/repository` `TestListUsersEscapesLikeWildcards`

#### Scenario: User enforcement: Paging boundaries
- **WHEN** 101 users exist and an admin lists with `page_size` 100, then with the returned cursor, and a third time with `page_size` 500
- **THEN** the first page holds 100 users and a non-empty `next_cursor`, the second holds 1 user and an empty `next_cursor`, and the third call is clamped to 100 users
- **VERIFIED BY**: team-identity `internal/repository` `TestListUsersPaging`

#### Scenario: User enforcement: Oversized query is rejected
- **WHEN** an admin lists with a query of 65 bytes
- **THEN** the call returns `InvalidArgument`
- **VERIFIED BY**: team-identity `internal/handler` `TestListUsersQueryTooLong`

#### Scenario: User enforcement: Listing never exposes credentials and is not audited
- **WHEN** an admin lists users
- **THEN** the serialized response contains no password hash and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/handler` `TestListUsersNoCredentialsNoAudit`

#### Scenario: User enforcement: Non-admin cannot list users
- **WHEN** a seller, and in a second case an anonymous caller, calls ListUsers
- **THEN** the first call returns `PermissionDenied` and the second returns `Unauthenticated`
- **VERIFIED BY**: team-identity `internal/handler` `TestListUsersRequiresAdminScope`
