## MODIFIED Requirements

### Requirement: The edge collector accepts browsing beacons and produces tracking events

The gateway SHALL expose a lightweight telemetry endpoint (`POST /api/track`) that accepts a
browser beacon describing a browsing action of type VIEW, CLICK, ADD_TO_CART, or IMPRESSION, maps
it to a `platform.analytics.v1.TrackingEvent` with the corresponding `EventType`, wraps it in a
`platform.events.v1.EventEnvelope` (`type = "platform.analytics.v1.TrackingEvent"`), and publishes
it to the Kafka topic `analytics.events`. Each event of a batch is validated on its own (see
`tracking-ingest-integrity`): invalid events are dropped, and only a body with no valid event is refused. The endpoint SHALL remain pure edge telemetry —
validate, stamp identity, forward — holding no business logic and owning no analytics storage
(architecture Rule 2).

#### Scenario: A browsing beacon becomes a tracking event on analytics.events

- **WHEN** the gateway receives a valid `POST /api/track` beacon for a product view
- **THEN** it publishes exactly one `EventEnvelope` to the `analytics.events` topic whose `type` is
  `platform.analytics.v1.TrackingEvent` and whose payload unmarshals to a `TrackingEvent` with
  `EventType = EVENT_TYPE_VIEW` and the beacon's behavioral context (listing id, session id,
  page/path)

#### Scenario: A malformed beacon is rejected without producing

- **WHEN** the gateway receives a beacon with no recognizable event type or a malformed body
- **THEN** it responds with a client error and publishes nothing to `analytics.events`
