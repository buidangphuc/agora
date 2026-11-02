## MODIFIED Requirements

### Requirement: Telemetry emission is best-effort and does not block browsing

The frontend SHALL fire the tracking beacon asynchronously (`navigator.sendBeacon`, or a
`keepalive` fetch fallback) so that recording an event never blocks or fails the user's browsing
action. The gateway producer SHALL be the Kafka producer in every environment (there is no no-op
producer and no `KAFKA_ENABLED` switch), SHALL log a produce failure without erroring the request,
and SHALL bound each publish by `TRACK_PUBLISH_TIMEOUT` (default 2 seconds).

#### Scenario: A browsing action still succeeds when the event cannot be delivered

- **WHEN** the buyer performs a tracked action (view, click, or add-to-cart) while the broker is unavailable
- **THEN** the browsing action completes normally and no user-visible error is shown
- **VERIFIED BY**: team-gateway `internal/edge` `TestTrackAcceptsBeaconWhenPublishFails`

#### Scenario: A stuck broker does not hold the browser request

- **WHEN** the broker accepts the connection but never acknowledges a record
- **THEN** the beacon is answered 202 within `TRACK_PUBLISH_TIMEOUT` plus 1 s
- **VERIFIED BY**: team-gateway `internal/edge` `TestTrackPublishIsBoundedByTimeout`
