## ADDED Requirements

### Requirement: An erased user's notifications, alerts and preferences are deleted

On `identity.events/UserErased` team-notification SHALL delete every notification, alert subscription and notification
preference row of the user and no row of another user.

#### Scenario: The user's notification data is deleted

- **WHEN** a `UserErased` event is consumed for a user with 4 notifications, 2 alert subscriptions and a preferences row
- **THEN** zero rows for that user remain in `notifications`, `alert_subscriptions` and `notification_prefs`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/notification/user_erasure.feature` `The user's notification data is deleted`

#### Scenario: Other users' notifications are untouched

- **WHEN** a `UserErased` event for user A is consumed and user B has notifications and alert subscriptions
- **THEN** every row of user B is unchanged and user B still lists their notifications
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/notification/user_erasure.feature` `Other users' notifications are untouched`

#### Scenario: A user with no notification data is a no-op

- **WHEN** a `UserErased` event is consumed for a user who has no rows
- **THEN** the handler returns no error and nothing is published to the DLQ
- **VERIFIED BY**: `team-notification/internal/store/erase_pg_test.go` `TestEraseUnknownUserIsNoOp`

#### Scenario: Erasure is reachable only through the identity topic

- **WHEN** a `UserErased` record is produced to a topic other than `identity.events`, or a user principal calls any team-notification RPC naming another user
- **THEN** no notification of the named user is deleted
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/notification/user_erasure.feature` `Erasure is reachable only through the identity topic`

### Requirement: The notification erasure consumer is idempotent and fail-visible

The team-notification consumer SHALL use its own consumer group and the DLQ `identity.events.notification.dlq`, SHALL
apply each event in one transaction, SHALL skip event types other than `UserErased`, and SHALL produce the same end
state on replay.

#### Scenario: A replayed event changes nothing

- **WHEN** the same `UserErased` event is delivered twice
- **THEN** the second delivery returns no error and no row of the user exists
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/notification/user_erasure.feature` `A replayed event changes nothing`

#### Scenario: A malformed event goes to the DLQ

- **WHEN** a record on `identity.events` is not a valid `EventEnvelope`, or a `UserErased` payload has an empty `user_id`
- **THEN** the record is published to `identity.events.notification.dlq` without retry and no row changes
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/notification/user_erasure.feature` `A malformed event goes to the DLQ`

#### Scenario: A store failure rolls back and is retried

- **WHEN** the erase transaction fails half-way because Postgres is unavailable
- **THEN** no table keeps a partial deletion, the record is retried up to 5 times, and the erasure completes after the database returns with the DLQ empty
- **VERIFIED BY**: `team-notification/internal/store/erase_pg_test.go` `TestEraseRollsBackOnFailure`

#### Scenario: Other identity events are ignored

- **WHEN** a `UserStatusChanged` event for a user is consumed
- **THEN** no notification of that user is deleted and nothing is published to the DLQ
- **VERIFIED BY**: `team-notification/internal/events/identity_test.go` `TestIgnoresOtherEventTypes`

#### Scenario: The erased id is not logged

- **WHEN** the consumer handles a `UserErased` event
- **THEN** the service log holds a count and a hashed id and no line contains the raw user id
- **VERIFIED BY**: `team-notification/internal/events/identity_test.go` `TestErasureLogsNoRawUserID`
