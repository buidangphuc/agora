## ADDED Requirements

### Requirement: TrackingEvent carries first-class attribution metadata

The system SHALL support first-class attribution fields on `platform.analytics.v1.TrackingEvent`: `placement_id` (string), `impression_id` (string), and `model_version` (string).

#### Scenario: Edge accepts beacon with placement attribution

- **WHEN** a client sends a tracking beacon with `placementId: "similar_items"`, `impressionId: "imp-uuid-123"`, and `modelVersion: "als_v1"` to `POST /api/track`
- **THEN** the gateway returns HTTP 204 No Content and publishes an `EventEnvelope` containing a `TrackingEvent` where `placement_id == "similar_items"`, `impression_id == "imp-uuid-123"`, and `model_version == "als_v1"` to Kafka topic `analytics.events`

#### Scenario: Warehouse consumer ingests attribution metadata

- **WHEN** the analytics worker processes an `analytics.events` message with attribution metadata
- **THEN** it records `placement_id`, `impression_id`, and `model_version` into the warehouse tracking record
