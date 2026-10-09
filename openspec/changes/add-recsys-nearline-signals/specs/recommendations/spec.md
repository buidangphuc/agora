## ADDED Requirements

### Requirement: Nearline real-time session signals

The system SHALL maintain real-time user session signals and recent interactions in Redis updated within seconds of tracking event receipt.

#### Scenario: User recent views update nearline signals

- **WHEN** a user views item `item-A` and then `item-B`
- **THEN** the nearline signal layer records `[item-B, item-A]` in the user's recent items list in Redis and increments the respective category affinities

#### Scenario: Real-time item co-occurrence is tracked

- **WHEN** multiple users view `item-A` and `item-B` within the same session
- **THEN** the co-view count between `item-A` and `item-B` is incremented in Redis

### Requirement: Nearline signals are consumed from analytics.events

A process (`python -m recsys.nearline`) SHALL consume the Kafka topic `analytics.events` as its own consumer group and
apply each tracking event to the Redis keys of the nearline layer. Views, clicks and add-to-carts feed the actor's
recents, category affinity and the session's co-views; impressions and clicks feed the position-debiased click-through
rate. The actor SHALL be the warehouse `user_key` (the principal's id for a signed-in user, else `anon:<anonymous_id>`).
It SHALL commit an offset only after the events it covers were written to Redis, so a Redis failure replays them. An
event delivered twice within 15 minutes, or older than the 24 hour window, SHALL change nothing. A message that is not
a tracking event or cannot be decoded SHALL be skipped and counted, not stop the consumer.

#### Scenario: Redelivered and stale events change nothing

- **WHEN** the same event id is delivered twice, and an event older than the window is delivered
- **THEN** the recents, category affinities, co-view counts and click-through counters are as after one delivery of the
  first and none of the second
- **VERIFIED BY**: platform-recsys/tests/test_nearline.py › test_a_redelivered_event_is_applied_once and test_events_older_than_the_window_are_ignored. Not verifiable end to end: the gateway stamps every event's id and time, so no public-edge call can redeliver an envelope or produce an old one.

#### Scenario: An undecodable message is skipped

- **WHEN** a message on `analytics.events` is not a protobuf message, and a valid tracking event follows it
- **THEN** the consumer counts the first as undecodable and applies the second
- **VERIFIED BY**: platform-recsys/tests/test_nearline_consumer.py › test_other_envelopes_and_garbage_are_skipped_not_fatal. Not verifiable end to end: the gateway only produces well-formed envelopes, and e2e has no way to write raw bytes to the topic through the public edge.

### Requirement: Nearline keys live outside the generation namespace

Nearline keys (`recs:nearline:*`) SHALL NOT be scoped to a model generation and SHALL NOT be written, moved or deleted
by publishing a generation, by retention or by rollback; they expire by their own TTL (24 hours). Their layout is the
contract team-ai reads (see `design.md`).

#### Scenario: A generation switch leaves nearline keys alone

- **WHEN** nearline keys exist and the recsys job promotes three generations in a row
- **THEN** the nearline keys are unchanged and keep their TTL
