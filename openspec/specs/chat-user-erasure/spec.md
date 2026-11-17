# chat-user-erasure Specification

## Purpose
TBD - created by archiving change content-user-erasure. Update Purpose after archive.

## Requirements

### Requirement: An erased user's chat messages are replaced by a placeholder

On `identity.events/UserErased` team-chat SHALL replace the text of every message the user sent with the fixed text
"Tin nhắn đã bị xóa", set the sender name to "Người dùng đã xóa", empty the rich payload, keep the thread and the
counterparty's messages, and replace `last_message_text` of a thread whose last message was the user's.

#### Scenario: The user's messages are replaced and the thread is kept

- **WHEN** a `UserErased` event is consumed for a buyer who sent 3 messages in a thread where the seller sent 2
- **THEN** the buyer's 3 messages read "Tin nhắn đã bị xóa" with sender name "Người dùng đã xóa", the seller's 2 messages are unchanged and the thread still lists for the seller
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `The user's messages are replaced and the thread is kept`

#### Scenario: A rich message payload is emptied

- **WHEN** a `UserErased` event is consumed for a user who sent an image message with a non-empty payload
- **THEN** that message's payload is empty and its text is "Tin nhắn đã bị xóa"
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `A rich message payload is emptied`

#### Scenario: The thread preview no longer shows erased text

- **WHEN** a `UserErased` event is consumed for the sender of the last message of a thread
- **THEN** the thread's `last_message_text` is "Tin nhắn đã bị xóa" and, where the counterparty sent the last message, `last_message_text` is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `The thread preview no longer shows erased text`

#### Scenario: Other users' messages are untouched

- **WHEN** a `UserErased` event for user A is consumed and user B sent messages in other threads
- **THEN** every message and thread of user B is unchanged
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `Other users' messages are untouched`

#### Scenario: Erasure is reachable only through the identity topic

- **WHEN** a `UserErased` record is produced to a topic other than `identity.events`, or a user principal calls any team-chat RPC naming another user
- **THEN** no message of the named user is replaced or changed
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `Erasure is reachable only through the identity topic`

### Requirement: The chat erasure consumer is idempotent and fail-visible

The team-chat consumer SHALL use its own consumer group and the DLQ `identity.events.chat.dlq`, SHALL apply each
event in one transaction, SHALL skip event types other than `UserErased`, and SHALL produce the same end state on replay.

#### Scenario: A replayed event changes nothing

- **WHEN** the same `UserErased` event is delivered twice
- **THEN** the second delivery returns no error and the messages equal the state after the first delivery
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `A replayed event changes nothing`

#### Scenario: A malformed event goes to the DLQ

- **WHEN** a record on `identity.events` is not a valid `EventEnvelope`, or a `UserErased` payload has an empty `user_id`
- **THEN** the record is published to `identity.events.chat.dlq` without retry and no message changes
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/chat/user_erasure.feature` `A malformed event goes to the DLQ`

#### Scenario: A store failure rolls back and is retried

- **WHEN** the erase transaction fails half-way because Postgres is unavailable
- **THEN** no message or thread keeps a partial change, the record is retried up to 5 times, and the erasure completes after the database returns with the DLQ empty
- **VERIFIED BY**: `team-chat/internal/store/erase_pg_test.go` `TestEraseRollsBackOnFailure`

#### Scenario: Other identity events are ignored

- **WHEN** a `UserStatusChanged` event for a user is consumed
- **THEN** no message of that user changes and nothing is published to the DLQ
- **VERIFIED BY**: `team-chat/internal/events/identity_test.go` `TestIdentityIgnoresOtherEventTypes`

#### Scenario: Message text and the erased id are not logged

- **WHEN** the consumer handles a `UserErased` event
- **THEN** the service log holds a count and a hashed id and no line contains the raw user id or any message text
- **VERIFIED BY**: `team-chat/internal/events/identity_test.go` `TestIdentityErasureLogsNoRawUserIDOrText`
