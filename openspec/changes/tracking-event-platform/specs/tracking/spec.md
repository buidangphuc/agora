## MODIFIED Requirements

### Requirement: Analytics tracking events are defined in the contract

The platform contract SHALL define a `platform.analytics.v1.TrackingEvent` message in
`platform-core/packages/proto/platform/analytics/v1/analytics.proto`, distinguishing the event types VIEW, CLICK,
ADD_TO_CART, IMPRESSION, PAGE_VIEW, SEARCH, REMOVE_FROM_CART, CHECKOUT_START, SHARE, IDENTIFY and CONSENT_UPDATE via
an `EventType` enum whose zero value is `EVENT_TYPE_UNSPECIFIED` (values prefixed `EVENT_TYPE_*` per buf
`ENUM_VALUE_PREFIX`). The message SHALL carry the behavioral context needed for warehousing and recommendation
(listing id, session id, anonymous id, page/path, referrer, result position, search query), the attribution fields
`placement_id`, `request_id` and `model_version`, the type-specific fields defined by the tracking taxonomy (dwell
time, result count, filters, quantity, share channel, client event id, client time, sample rate, page type) plus a
`map<string,string> properties` extension field, and SHALL NOT carry authenticated user identity in its own fields
(identity travels in the envelope's `principal`). Every field SHALL be added with a new field number; none is
renumbered or removed.

#### Scenario: Contract defines the four tracking event types

- **WHEN** a producer needs to record a product view, click, add-to-cart, or search impression
- **THEN** it can construct a single `platform.analytics.v1.TrackingEvent` with the corresponding
  `EventType`, without any new message type per event

#### Scenario: Contract defines the extended event types with attribution

- **WHEN** a producer needs to record a page view, a search, a cart removal, a checkout start, a share, a login
  identify or a consent change, or an impression attributed to a placement, serving request and model version
- **THEN** it can construct a single `TrackingEvent` with the corresponding `EventType` and the attribution fields,
  and `buf breaking` against the previous contract passes

### Requirement: The edge collector accepts browsing beacons and produces tracking events

The gateway SHALL expose a lightweight telemetry endpoint (`POST /api/track`) that accepts a browser beacon (a single
event object, a JSON array of events, or a batch object carrying `sent_at` and an `events` array) describing browsing
actions of any type registered in the tracking taxonomy, validates each event against the taxonomy, maps each valid
event to a `platform.analytics.v1.TrackingEvent` with the corresponding `EventType`, wraps it in a
`platform.events.v1.EventEnvelope` (`type = "platform.analytics.v1.TrackingEvent"`, `schema_version` set), and
publishes it to the Kafka topic `analytics.events`; invalid events are dropped individually and counted. The endpoint
SHALL remain pure edge telemetry — validate, stamp identity and time, scrub, forward — holding no business logic and
owning no analytics storage (architecture Rule 2).

#### Scenario: A browsing beacon becomes a tracking event on analytics.events

- **WHEN** the gateway receives a valid `POST /api/track` beacon for a product view
- **THEN** it publishes exactly one `EventEnvelope` to the `analytics.events` topic whose `type` is
  `platform.analytics.v1.TrackingEvent` and whose payload unmarshals to a `TrackingEvent` with
  `EventType = EVENT_TYPE_VIEW` and the beacon's behavioral context (listing id, session id,
  page/path)

#### Scenario: A malformed beacon is rejected without producing

- **WHEN** the gateway receives a beacon with no recognizable event type or a malformed body
- **THEN** it responds with a client error and publishes nothing to `analytics.events`

#### Scenario: A batch mixes valid and invalid events

- **WHEN** the gateway receives a batch of a valid `SEARCH`, a valid `IMPRESSION` with attribution and an event of
  unknown type
- **THEN** it publishes exactly two envelopes, responds 204, and counts one drop with reason `unknown_type`
