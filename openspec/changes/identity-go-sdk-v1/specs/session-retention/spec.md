## MODIFIED Requirements

### Requirement: The sweeper runs hourly in bounded batches without blocking live traffic

The sweeper SHALL always run, with no enable variable, once at start and then every `SESSION_SWEEP_INTERVAL` (default 1h) as a `lifecycle.Every` component that the process stops before it closes the pool, delete at most `SESSION_SWEEP_BATCH_SIZE` (default 500) rows per statement, one batch per transaction, and repeat until a batch is short. It SHALL skip rows locked by other transactions.

#### Scenario: Session retention: batches of 500
- **WHEN** 1,200 eligible sessions exist and one sweep runs
- **THEN** exactly 3 delete batches run (500, 500, 200) and 0 eligible rows remain
- **VERIFIED BY**: team-identity `internal/identity` `TestSweepOnceBatches`

#### Scenario: Session retention: locked row is skipped
- **WHEN** a concurrent transaction holds `FOR UPDATE` on an eligible session and a sweep runs
- **THEN** the sweep completes within 2 s without waiting and leaves that row for the next sweep
- **VERIFIED BY**: team-identity `internal/store` `TestPurgeSessionsSkipsLocked`

#### Scenario: Session retention: sweep error is logged and retried
- **WHEN** a tick runs and the purge returns a DB error
- **THEN** the sweeper logs a warning, keeps running, and the next tick purges normally; readiness stays SERVING
- **VERIFIED BY**: team-identity `internal/identity` `TestSweeperSurvivesError`

#### Scenario: Session retention: shutdown drains the sweeper
- **WHEN** the process context is cancelled during a sweep
- **THEN** the sweeper returns before the pool closes, no error is logged, and `Run` returns nil
- **VERIFIED BY**: team-identity `internal/app` `TestShutdownStopsSweeperBeforePoolCloses`

#### Scenario: Session retention: the sweeper always runs
- **WHEN** the service starts with a database holding one purgeable session and `SESSION_SWEEP_INTERVAL=1h`
- **THEN** the session row is gone within 5 s of start, because the first sweep runs at start and no variable switches the sweeper off
- **VERIFIED BY**: team-identity `internal/app` `TestSweeperRunsAtStart`

#### Scenario: Session retention: purge does not log personal data
- **WHEN** a sweep deletes rows
- **THEN** captured log lines contain only counts and durations, never an ip, device, user id or hash
- **VERIFIED BY**: team-identity `internal/identity` `TestSweeperLogsCountsOnly`

### Requirement: Retention and cadence are configurable and validated

team-identity SHALL read `SESSION_RETENTION` (720h), `LOGIN_HISTORY_RETENTION` (4320h), `SESSION_SWEEP_INTERVAL` (1h), `SESSION_SWEEP_BATCH_SIZE` (500), list them in `.env.example` and the README configuration table, SHALL NOT read `SESSION_SWEEP_ENABLED`, and SHALL refuse to start on non-positive or unparsable values naming the variable.

#### Scenario: Session retention: defaults
- **WHEN** none of the variables is set
- **THEN** config load yields 720h, 4320h, 1h and 500
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionDefaults`

#### Scenario: Session retention: invalid value is refused
- **WHEN** config loads with `SESSION_RETENTION=0`, `SESSION_RETENTION=abc`, or `SESSION_SWEEP_BATCH_SIZE=0`
- **THEN** config load returns an error naming the variable
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionInvalidRejected`

#### Scenario: Session retention: env example matches config
- **WHEN** `make check-env` runs
- **THEN** it exits 0 with all four variables present in `.env.example` and no `SESSION_SWEEP_ENABLED`
- **VERIFIED BY**: team-identity `make check-env`

#### Scenario: Session retention: disabled sweeper
- **WHEN** the environment sets `SESSION_SWEEP_ENABLED=false` and the service starts
- **THEN** config load succeeds, the declared keys do not include `SESSION_SWEEP_ENABLED`, and the first sweep still runs
- **VERIFIED BY**: team-identity `internal/config` `TestRetentionHasNoEnableFlag`
