# loop-event-storage Specification

## Purpose
team-analytics stores each new loop event type under its lowercase name with its properties preserved.

## Requirements

### Requirement: The warehouse stores each new loop event type under its lowercase name

The tracking consumer SHALL store a TrackingEvent whose EventType is `EVENT_TYPE_SIGN_UP`, `EVENT_TYPE_LOGIN`, `EVENT_TYPE_FOLLOW_SELLER`, `EVENT_TYPE_REFERRAL_REDEEM`, `EVENT_TYPE_DAILY_CHECK_IN`, `EVENT_TYPE_ALERT_SUBSCRIBE` or `EVENT_TYPE_AI_INTERACTION` with `event_type` equal to `sign_up`, `login`, `follow_seller`, `referral_redeem`, `daily_check_in`, `alert_subscribe` or `ai_interaction` respectively.

#### Scenario: Loop storage: each new type is stored under its name
- **WHEN** a TrackingEvent of each of the seven new EventType values is mapped by the consumer
- **THEN** the record `EventType` equals the matching lowercase name for all seven, and none equals "unspecified"
- **VERIFIED BY**: team-analytics `internal/events` `TestNewLoopEventTypesMapToNames`

#### Scenario: Loop storage: a produced event of each new type reaches the warehouse
- **WHEN** one TrackingEvent envelope of each new type, each with a unique event id and session id, is produced to analytics.events
- **THEN** the warehouse holds exactly one row per event id whose `event_type` is the matching lowercase name
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_storage.feature` "Loop storage: a produced event of each new type reaches the warehouse"

#### Scenario: Loop storage: existing types keep their names
- **WHEN** a TrackingEvent of each existing EventType value 1 to 14 is mapped
- **THEN** each record `EventType` equals the name it had before this change (for example "view", "share", "favorite", "purchase")
- **VERIFIED BY**: team-analytics `internal/events` `TestExistingEventTypeNamesUnchanged`

#### Scenario: Loop storage: an unrecognised enum value is stored as unspecified without failing
- **WHEN** a TrackingEvent carries an EventType number outside the known set (for example 99)
- **THEN** the record `EventType` is "unspecified" and the consumer returns no error
- **VERIFIED BY**: team-analytics `internal/events` `TestUnrecognisedEventTypeIsUnspecified`

### Requirement: Properties of the new types are preserved

The consumer SHALL persist the `properties` map, `listing_id`, session and anonymous ids and the envelope attribution of a new-type event exactly as received, and SHALL apply the existing consent rules to it.

#### Scenario: Loop storage: properties and listing id are preserved
- **WHEN** an `ai_interaction` event with properties `surface=assistant`, `action=accepted` and listing id "l-1", and an `alert_subscribe` event with `alert_type=price_drop`, are mapped
- **THEN** each record carries exactly those properties and listing id, and the principal id of the envelope
- **VERIFIED BY**: team-analytics `internal/events` `TestNewLoopEventsPreservePropertiesAndPrincipal`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_storage.feature` "Loop storage: properties and listing id are preserved"

#### Scenario: Loop storage: an event with no properties is stored with empty properties
- **WHEN** a `referral_redeem` event with no properties is consumed
- **THEN** its row exists with the `properties` column holding the JSON text `{}` (not NULL), as both the DuckDB and BigQuery writers store an empty map, and no error is raised
- **VERIFIED BY**: team-analytics `internal/events` `TestReferralRedeemWithoutPropertiesIsStored`

#### Scenario: Loop storage: the client id is blanked without analytics consent
- **WHEN** a `sign_up` event carries a ga_client_id and a consent snapshot with analytics false
- **THEN** the stored `ga_client_id` is empty
- **VERIFIED BY**: team-analytics `internal/events` `TestNewLoopEventClientIdBlankedWithoutConsent`
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_storage.feature` "Loop storage: the client id is blanked without analytics consent"

#### Scenario: Loop storage: a replayed event is stored once
- **WHEN** the same `login` envelope (same event id) is produced to analytics.events twice
- **THEN** the warehouse holds exactly one row for that event id after a 10 second settle (same pattern as `features/tracking/ingest_idempotency.feature`)
- **VERIFIED BY**: platform-e2e `features/tracking/loop_event_storage.feature` "Loop storage: a replayed event is stored once"

### Requirement: Non-tracking envelopes are still skipped

The tracking consumer SHALL continue to skip non-tracking envelopes without error.

#### Scenario: Loop storage: a non-tracking envelope is still skipped
- **WHEN** an envelope whose type is not `platform.analytics.v1.TrackingEvent` is consumed
- **THEN** no row is written and the consumer reports no error
- **VERIFIED BY**: team-analytics `internal/events` `TestNonTrackingEnvelopeIsSkipped`

#### Scenario: Loop storage: an undecodable payload is still a consumer error
- **WHEN** an envelope of type `platform.analytics.v1.TrackingEvent` carries a payload that cannot be decoded
- **THEN** the consumer returns a decode error and writes no row
- **VERIFIED BY**: team-analytics `internal/events` `TestMalformedEnvelopeErrors`
