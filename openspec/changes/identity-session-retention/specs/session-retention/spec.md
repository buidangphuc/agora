## ADDED Requirements

### Requirement: Sessions are purged 30 days after revoke or refresh expiry

The identity sweeper SHALL delete `sessions` rows that are revoked or past their refresh expiry (or the 90-day absolute max age) for longer than `SESSION_RETENTION` (default 720h). Revoked rows are aged from `COALESCE(revoked_at, last_seen)`; other rows from `LEAST(COALESCE(refresh_expires_at, created_at + 90 days), created_at + 90 days)`. A session that is not revoked and whose effective expiry is in the future SHALL never be deleted or modified.

#### Scenario: Session retention: revoked session older than retention is deleted
- **WHEN** a session revoked 31 days ago exists and a sweep runs
- **THEN** its row is gone (0 rows with that id) and the sweep reports it in its deleted count
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsDeletesOldRevoked`

#### Scenario: Session retention: expired refresh session is deleted
- **WHEN** an un-revoked session has `refresh_expires_at` 31 days in the past and a sweep runs
- **THEN** its row is deleted
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsDeletesExpiredRefresh`

#### Scenario: Session retention: absolute max age counts as expiry
- **WHEN** an un-revoked session has `created_at` 121 days ago (90-day absolute max plus 31 days) and a `refresh_expires_at` still in the future, and a sweep runs
- **THEN** its row is deleted
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsAbsoluteMaxAge`

#### Scenario: Session retention: legacy rows without timestamps
- **WHEN** a revoked session has NULL `revoked_at` and `last_seen` 31 days ago, and an un-revoked legacy session has NULL `refresh_expires_at` and `created_at` 121 days ago
- **THEN** both rows are deleted
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsLegacyRows`

#### Scenario: Session retention: active session is never touched
- **WHEN** a sweep runs while the DB holds an un-revoked session with `refresh_expires_at` in the future and `created_at` 80 days ago, and a session whose refresh expired yesterday
- **THEN** both rows still exist and their columns are unchanged
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsKeepsActive`

#### Scenario: Session retention: boundary at exactly the retention
- **WHEN** sessions ended `SESSION_RETENTION` minus 1 s, exactly `SESSION_RETENTION`, and `SESSION_RETENTION` plus 1 s ago, and a sweep runs
- **THEN** only the plus-1-s session is deleted (deletion needs strictly older)
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsRetentionBoundary`

#### Scenario: Session retention: recently revoked session is kept
- **WHEN** a session was revoked 29 days ago and a sweep runs
- **THEN** its row still exists with `revoked = true`
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsKeepsRecentRevoked`

#### Scenario: Session retention: revoke records revoked_at
- **WHEN** a session is revoked by the user, by reuse detection, and by the absolute-lifetime check
- **THEN** each row has `revoked_at` within 5 s of now
- **VERIFIED BY**: team-identity `internal/repository` `TestRevokePathsSetRevokedAt`

### Requirement: Login history is purged after 180 days

The sweeper SHALL delete `login_history` rows whose `created_at` is older than `LOGIN_HISTORY_RETENTION` (default 4320h). Newer rows SHALL be untouched.

#### Scenario: Session retention: old login history is deleted
- **WHEN** login_history holds rows aged 181 days and 179 days and a sweep runs
- **THEN** only the 181-day row is deleted and `ListLoginHistory` for that user returns the 179-day row
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeLoginHistory`

#### Scenario: Session retention: login history without rows or with a failed purge
- **WHEN** login_history is empty, or the delete fails with a DB error
- **THEN** the sweep returns 0 deleted without panic, and session purging still ran in the same pass
- **VERIFIED BY**: team-identity `internal/service` `TestSweepOnceLoginHistoryErrorIsolated`

#### Scenario: Session retention: login history boundary
- **WHEN** a row is exactly 180 days old at sweep time
- **THEN** the row is kept
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeLoginHistoryBoundary`

### Requirement: The sweeper runs hourly in bounded batches without blocking live traffic

The sweeper SHALL run every `SESSION_SWEEP_INTERVAL` (default 1h), delete at most `SESSION_SWEEP_BATCH_SIZE` (default 500) rows per statement, one batch per transaction, and repeat until a batch is short. It SHALL skip rows locked by other transactions.

#### Scenario: Session retention: batches of 500
- **WHEN** 1,200 eligible sessions exist and one sweep runs
- **THEN** exactly 3 delete batches run (500, 500, 200) and 0 eligible rows remain
- **VERIFIED BY**: team-identity `internal/service` `TestSweepOnceBatches`

#### Scenario: Session retention: locked row is skipped
- **WHEN** a concurrent transaction holds `FOR UPDATE` on an eligible session and a sweep runs
- **THEN** the sweep completes within 2 s without waiting and leaves that row for the next sweep
- **VERIFIED BY**: team-identity `internal/repository` `TestPurgeSessionsSkipsLocked`

#### Scenario: Session retention: sweep error is logged and retried
- **WHEN** a tick runs and the purge returns a DB error
- **THEN** the sweeper logs a warning, keeps running, and the next tick purges normally; readiness stays SERVING
- **VERIFIED BY**: team-identity `internal/service` `TestSweeperSurvivesError`

#### Scenario: Session retention: shutdown drains the sweeper
- **WHEN** `CloseResources` runs during a sweep
- **THEN** the sweeper goroutine exits before the pool closes and no error is logged
- **VERIFIED BY**: team-identity `internal/bootstrap` `TestCloseResourcesStopsSweeper`

#### Scenario: Session retention: purge does not log personal data
- **WHEN** a sweep deletes rows
- **THEN** captured log lines contain only counts and durations, never an ip, device, user id or hash
- **VERIFIED BY**: team-identity `internal/service` `TestSweeperLogsCountsOnly`

### Requirement: Retention and cadence are configurable and validated

team-identity SHALL read `SESSION_RETENTION` (720h), `LOGIN_HISTORY_RETENTION` (4320h), `SESSION_SWEEP_INTERVAL` (1h), `SESSION_SWEEP_BATCH_SIZE` (500) and `SESSION_SWEEP_ENABLED` (true), list them in `.env.example` and the README configuration table, and refuse to start on non-positive or unparsable values.

#### Scenario: Session retention: defaults
- **WHEN** none of the variables is set
- **THEN** config load yields 720h, 4320h, 1h, 500 and enabled
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionDefaults`

#### Scenario: Session retention: invalid value is refused
- **WHEN** config loads with `SESSION_RETENTION=0`, `SESSION_RETENTION=abc`, or `SESSION_SWEEP_BATCH_SIZE=0`
- **THEN** config load returns an error naming the variable
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionInvalidRejected`

#### Scenario: Session retention: env example matches config
- **WHEN** `make check-env` runs
- **THEN** it exits 0 with all five variables present in `.env.example`
- **VERIFIED BY**: team-identity `make check-env`

#### Scenario: Session retention: disabled sweeper
- **WHEN** `SESSION_SWEEP_ENABLED=false`
- **THEN** no sweeper goroutine starts and no row is deleted after 2 intervals
- **VERIFIED BY**: team-identity `internal/bootstrap` `TestSweeperDisabled`

### Requirement: Retention is observable end to end

With the short-retention compose override the running service SHALL purge revoked sessions and old login history within seconds, via the real database.

#### Scenario: Session retention: revoked session disappears end to end
- **WHEN** a user logs in on two devices, revokes one via `RevokeSession`, and waits 12 s on the short-retention stack
- **THEN** `ListSessions` returns only the active session (revoked row gone from `identity_db`) and the active session still accepts `RefreshSession`

#### Scenario: Session retention: old login history disappears end to end
- **WHEN** a user logs in once on the short-retention stack and waits 12 s
- **THEN** `ListLoginHistory` for that user no longer contains that login entry

#### Scenario: Session retention: default stack keeps rows
- **WHEN** a user logs in and revokes a session on the default stack (no override) and 12 s pass
- **THEN** `ListSessions` still returns the revoked session
