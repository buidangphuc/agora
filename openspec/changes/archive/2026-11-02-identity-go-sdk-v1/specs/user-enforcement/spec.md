## MODIFIED Requirements

### Requirement: Admin sets a user's status, audit first

`AccountAdminService.SetUserStatus` SHALL, for a caller holding scope `admin`, validate the request, load the target user, write one team-audit event, and then in one database transaction update the user's status, revoke every active session of the user (for target status `restricted`, `suspended` or `banned`) and enqueue the outbox events. The audit event SHALL have `actor_id` equal to the caller's verified principal id, `action` `user.set_status`, `target_type` `user`, `target_id` the user id and metadata `status`, `reason_code`, `note` (when given) and `until` (RFC 3339 UTC, when given). The event SHALL be written, through the SDK audit gate, as service principal `service-team-identity` holding `audit.write`. `UPSTREAM_AUDIT_ADDR` is required in every ENV (no fallback mode). When the write fails or times out, `SetUserStatus` SHALL return `Unavailable` with the message `audit trail unavailable` and SHALL change nothing. The response SHALL carry the updated `AdminUser` and `sessions_revoked`. `reason_code` SHALL be 1 to 64 bytes of lower_snake_case (`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`). Re-setting the same `restricted`, `suspended` or `banned` status SHALL revoke every active session again. Setting `active` SHALL revoke no session, SHALL store the new `reason_code` in `status_reason` and SHALL clear `status_until`.

#### Scenario: User enforcement: Ban writes one audit event and revokes sessions
- **WHEN** an admin bans a buyer who has 3 active sessions with reason code `scam`
- **THEN** the RPC succeeds with `sessions_revoked` = 3, the user has status `banned` and `status_reason` `scam`, and team-audit holds exactly 1 event with the admin id as `actor_id`, `action` `user.set_status`, `target_type` `user`, the user id as `target_id` and metadata `status` = `banned`
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusBanAuditsAndRevokes`

#### Scenario: User enforcement: Audit is written before the change
- **WHEN** an admin calls SetUserStatus and the audit write is recorded by a fake writer
- **THEN** the fake receives the audit call before the repository transaction is invoked and the event carries the target id from the stored user and the actor id from the principal, not from the request
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusAuditPrecedesChange`

#### Scenario: User enforcement: Audit down refuses the change
- **WHEN** team-audit returns an error, and in a second case a deadline-exceeded, while an admin suspends a user
- **THEN** each case returns `Unavailable` with the message `audit trail unavailable`, the user row and all sessions are unchanged, and no outbox row is written
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusAuditDownReturnsUnavailable`

#### Scenario: User enforcement: No audit upstream configured
- **WHEN** `Load` runs with `ENV=local`, and again with `ENV=production`, each with `UPSTREAM_AUDIT_ADDR` unset
- **THEN** both calls return an error naming `UPSTREAM_AUDIT_ADDR`, so no service without an audit writer ever serves `SetUserStatus`
- **VERIFIED BY**: team-identity `internal/config` `TestAuditAddrRequiredInEveryEnv`

#### Scenario: User enforcement: Audit is called as the service principal
- **WHEN** the audit client writes an event
- **THEN** the outgoing metadata carries principal id `service-team-identity`, type `service` and scope `audit.write`, and no caller principal is forwarded
- **VERIFIED BY**: team-identity `internal/upstream` `TestAuditUsesServicePrincipal`

#### Scenario: User enforcement: Change failure after audit keeps the audit row
- **WHEN** the audit write succeeds and the transaction then fails with an internal error
- **THEN** the RPC returns `Internal`, the user row and sessions are unchanged, no outbox row exists, and the audit event already written is not retracted
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusTxFailureAfterAudit`

#### Scenario: User enforcement: Status change and revokes are atomic
- **WHEN** the outbox insert for the second `SessionRevoked` fails during a suspend of a user with 3 sessions
- **THEN** the user status is unchanged, all 3 sessions are still active and no outbox row exists
- **VERIFIED BY**: team-identity `internal/store` `TestSetUserStatusRollsBackOnOutboxFailure`

#### Scenario: User enforcement: Restrict revokes sessions and keeps the user able to log in
- **WHEN** an admin restricts a seller with 2 active sessions
- **THEN** `sessions_revoked` is 2, status is `restricted`, and a subsequent Login with the correct password succeeds
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusRestrictRevokesSessions`

#### Scenario: User enforcement: Reinstating to active revokes nothing
- **WHEN** an admin sets a restricted user with 1 active session to `active` with reason code `appeal_accepted`, and in a second case sets a suspended user with an `until` to `active`
- **THEN** `sessions_revoked` is 0, the session stays active, and the user has status `active`, `status_reason` `appeal_accepted`, no `status_until` and `status_changed_at` updated
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusActiveRevokesNothing`

#### Scenario: User enforcement: Setting the same status updates reason and until
- **WHEN** an admin suspends an already suspended user who has 1 active session with a later `until`
- **THEN** the call succeeds with `sessions_revoked` = 1, `status_until` holds the new value, one audit event is written, 1 `SessionRevoked` row and one `UserStatusChanged` with `previous_status` `suspended` are enqueued
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusSuspendExtend`

#### Scenario: User enforcement: Unknown user is NotFound without audit
- **WHEN** an admin calls SetUserStatus with a user id that does not exist
- **THEN** the RPC returns `NotFound` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusNotFoundNoAudit`

#### Scenario: User enforcement: Admin cannot change their own status
- **WHEN** an admin calls SetUserStatus with their own user id
- **THEN** the RPC returns `FailedPrecondition`, the fake audit writer receives 0 calls and the user row is unchanged
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusSelfRefused`

#### Scenario: User enforcement: Admin cannot change another admin
- **WHEN** an admin calls SetUserStatus on a user holding role `admin`
- **THEN** the RPC returns `FailedPrecondition`, the fake audit writer receives 0 calls and the user row is unchanged
- **VERIFIED BY**: team-identity `internal/identity` `TestSetUserStatusAdminTargetRefused`

#### Scenario: User enforcement: Invalid input is rejected before any audit
- **WHEN** an admin calls SetUserStatus with, in turn, status UNSPECIFIED, an empty `reason_code`, a `reason_code` of 65 bytes, the `reason_code` values `Scam`, `scam-x`, `1scam`, `scam_`, `_scam`, `scam__x` and `scam x` (not lower_snake_case), a `note` of 501 characters, an empty `user_id`, `SUSPENDED` without `until`, `SUSPENDED` with `until` equal to now, `SUSPENDED` with `until` in the past, and `BANNED` with `until` set
- **THEN** each call returns `InvalidArgument` and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusInvalidInput`

#### Scenario: User enforcement: Input boundaries are accepted
- **WHEN** an admin calls SetUserStatus with a 64-byte lower_snake_case `reason_code`, the reason codes `a` and `counterfeit_seller2`, a 500-character `note`, and `SUSPENDED` with `until` one second after now
- **THEN** the call succeeds and the audit event carries the full note
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusInputBoundaries`

#### Scenario: User enforcement: Non-admin cannot set a status
- **WHEN** a caller holding the seller scopes, and in a second case a caller with no principal, calls SetUserStatus
- **THEN** the first call returns `PermissionDenied`, the second returns `Unauthenticated`, and the fake audit writer receives 0 calls
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusRequiresAdminScope`

#### Scenario: User enforcement: Audit metadata never leaks into logs
- **WHEN** SetUserStatus runs with log and span capture enabled and a note containing a marker string
- **THEN** zero captured log lines and span attributes contain the note
- **VERIFIED BY**: team-identity `internal/rpc` `TestSetUserStatusNoteNeverLogged`
