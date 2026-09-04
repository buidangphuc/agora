## ADDED Requirements

### Requirement: The edge collector accepts browsing beacons and produces tracking events

The gateway SHALL expose a lightweight telemetry endpoint (`POST /api/track`) that accepts a
browser beacon describing a browsing action of type VIEW, CLICK, ADD_TO_CART, or IMPRESSION, maps
it to a `platform.analytics.v1.TrackingEvent` with the corresponding `EventType`, wraps it in a
`platform.events.v1.EventEnvelope` (`type = "platform.analytics.v1.TrackingEvent"`), and publishes
it to the Kafka topic `analytics.events`. The endpoint SHALL remain pure edge telemetry —
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

### Requirement: Tracking events carry the resolved principal, never PII in the payload

The gateway SHALL set the `EventEnvelope.principal` from the edge-resolved principal (the same
`x-principal-*` identity resolved for every request; anonymous callers yield the anonymous
principal) and SHALL NOT copy authenticated user identity into `TrackingEvent` fields. The beacon
body SHALL carry only behavioral context (listing id, session id, anonymous id, page/path,
referrer, result position, query, and a `properties` map).

#### Scenario: Authenticated browsing attributes the event via the envelope

- **WHEN** a logged-in buyer's beacon is collected at the gateway
- **THEN** the produced `EventEnvelope.principal` identifies the buyer while the `TrackingEvent`
  payload contains no authenticated user id in its own fields

### Requirement: Telemetry emission is best-effort and does not block browsing

The frontend SHALL fire the tracking beacon asynchronously (`navigator.sendBeacon`, or a
`keepalive` fetch fallback) so that recording an event never blocks or fails the user's browsing
action, and the gateway producer SHALL degrade to a no-op when Kafka is disabled
(`KAFKA_ENABLED=false`) without erroring the request.

#### Scenario: A browsing action still succeeds when the event cannot be delivered

- **WHEN** the buyer performs a tracked action (view, click, or add-to-cart) while the analytics
  producer is disabled or the broker is unavailable
- **THEN** the browsing action completes normally and no user-visible error is shown
