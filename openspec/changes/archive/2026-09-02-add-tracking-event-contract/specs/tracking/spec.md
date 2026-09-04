## ADDED Requirements

### Requirement: Analytics tracking events are defined in the contract

The platform contract SHALL define a `platform.analytics.v1.TrackingEvent` message in
`platform-core/packages/proto/platform/analytics/v1/analytics.proto`, distinguishing at least the
event types VIEW, CLICK, ADD_TO_CART, and IMPRESSION via an `EventType` enum whose zero value is
`EVENT_TYPE_UNSPECIFIED` (values prefixed `EVENT_TYPE_*` per buf `ENUM_VALUE_PREFIX`). The message
SHALL carry the behavioral context needed for warehousing and recommendation (listing id, session
id, anonymous id, page/path, referrer, result position, search query) plus a
`map<string,string> properties` extension field, and SHALL NOT carry authenticated user identity in
its own fields (identity travels in the envelope's `principal`).

#### Scenario: Contract defines the four tracking event types

- **WHEN** a producer needs to record a product view, click, add-to-cart, or search impression
- **THEN** it can construct a single `platform.analytics.v1.TrackingEvent` with the corresponding
  `EventType`, without any new message type per event

### Requirement: Tracking events reuse the standard event envelope and a dedicated topic

The system SHALL transport tracking events using the existing `platform.events.v1.EventEnvelope`
(the `TrackingEvent` serialized into `payload`, with `type = "platform.analytics.v1.TrackingEvent"`),
on the dedicated Kafka topic **`analytics.events`** — separate from `listing.events` and from any
transactional database. The contract change SHALL be additive and non-breaking.

#### Scenario: Tracking event rides the shared envelope on its own topic

- **WHEN** the contract is generated in a consumer repo and a `TrackingEvent` is wrapped in an
  `EventEnvelope`
- **THEN** the envelope's `type` field identifies it as `platform.analytics.v1.TrackingEvent`, the
  payload round-trips (marshal/unmarshal) unchanged, and the agreed transport topic is `analytics.events`

#### Scenario: Contract passes lint and breaking-change checks

- **WHEN** `buf lint` and `buf breaking` run against `platform-core/packages/proto`
- **THEN** both pass — the new `platform.analytics.v1` package introduces no lint violations and no
  breaking change to any existing package
