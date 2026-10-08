## Purpose

Defines the versioned behavioural event catalogue owned by team-analytics: which events exist, what every field
means, which fields are required, how each field is classified for privacy, how placements are named, and how
attribution (placement, serving request, model version, position) travels from a serving response into events.

## ADDED Requirements

### Requirement: The taxonomy is a single versioned catalogue owned by team-analytics

The system SHALL publish one machine-readable event taxonomy (version `v1`) in `platform-core`, owned by
team-analytics, listing for every client event type its name, purpose, required fields, optional fields, and for
every field its type, definition, maximum length and PII class. The taxonomy SHALL cover at least `PAGE_VIEW`,
`VIEW`, `IMPRESSION`, `CLICK`, `SEARCH`, `ADD_TO_CART`, `REMOVE_FROM_CART`, `CHECKOUT_START`, `SHARE`, `IDENTIFY`
and `CONSENT_UPDATE`, and every field of `platform.analytics.v1.TrackingEvent`. The proto enum and the catalogue
SHALL never disagree.

#### Scenario: Every proto event type is described in the catalogue

- **WHEN** the taxonomy consistency check runs against `analytics.proto` and the catalogue
- **THEN** every non-zero `EventType` value has a catalogue entry and every catalogue entry maps to an
  `EventType` value, otherwise the check fails

#### Scenario: Every field has a PII class

- **WHEN** a `TrackingEvent` field is added to the proto without a catalogue entry carrying a PII class
- **THEN** the taxonomy consistency check fails and names the field

### Requirement: Taxonomy and contract evolve additively with an explicit schema version

The contract SHALL evolve only additively (new enum values, new fields with new numbers; no renumbering, removal or
type change), verified by `buf breaking`. Every produced `EventEnvelope` SHALL carry `schema_version`, the taxonomy
version its payload was validated against. A consumer SHALL accept any `schema_version` up to the newest it knows
and SHALL count (not fail on) events carrying a newer version.

#### Scenario: Events carry the schema version they were validated against

- **WHEN** the gateway accepts a beacon under taxonomy `v1`
- **THEN** the produced envelope has `schema_version = 1`

#### Scenario: A newer schema version is tolerated

- **WHEN** the analytics consumer receives a tracking envelope with a `schema_version` higher than it knows
- **THEN** it stores the known fields, increments a schema-ahead counter, and does not stall the partition

### Requirement: Placements are named `<surface>.<slot>` from a registered list

Every surface that shows a ranked or curated list of listings SHALL have a `placement_id` of the form
`<surface>.<slot>` (lowercase `a-z`, digits and `_`, one dot). The v1 registry SHALL include `home.for_you`,
`home.trending`, `home.recently_viewed`, `home.flash_sale`, `search.results`, `pdp.similar`, `cart.cross_sell` and
`assistant.suggestions`. A new placement SHALL be registered in the taxonomy before it is used.

#### Scenario: An unregistered placement is rejected

- **WHEN** a beacon carries an impression with `placement_id = "home.mystery"` that is not in the registry
- **THEN** that event is dropped with reason `unknown_placement` and is not produced

#### Scenario: A malformed placement id is rejected

- **WHEN** a beacon carries `placement_id = "Home For You"`
- **THEN** that event is dropped with reason `invalid_field` and is not produced

### Requirement: List-scoped events carry full attribution

Every `IMPRESSION` and every `CLICK` on a listing shown inside a placement SHALL carry `placement_id`,
`request_id`, `model_version` (empty string allowed only when the serving response carried none), and a 1-based
`position`. `ADD_TO_CART` and `VIEW` that follow a click from a placement SHALL carry the same attribution when the
client still holds it for that listing in the session.

#### Scenario: An impression without attribution is rejected

- **WHEN** a beacon carries an `IMPRESSION` with a listing id but no `placement_id` or no `request_id` or
  `position = 0`
- **THEN** that event is dropped with reason `missing_required` and is not produced

#### Scenario: A click carries the same attribution as its impression

- **WHEN** a visitor is shown a `home.for_you` row whose serving response had request id R and model version M,
  and clicks the third card
- **THEN** the warehouse holds an impression and a click for that listing, both with `placement_id = home.for_you`,
  `request_id = R`, `model_version = M` and `position = 3`

### Requirement: Attribution flows from the serving response to the client and back into events

A serving response for a placement (`Recommend`, `SearchListings`) SHALL expose a `request_id` unique per call and
the `model_version` that produced it; `Recommend` SHALL also echo the `placement_id` it was asked for. The frontend
SHALL pass these values unchanged to every impression and click emitted for that rendered list. When a serving
response carries no `request_id`, the frontend SHALL mint one per rendered list (prefixed `fe-`) so the
impression-to-click join still holds.

#### Scenario: Search results are attributed to their search request

- **WHEN** a buyer searches and two results are seen and one is clicked
- **THEN** the two impressions and the click share one non-empty `request_id`, `placement_id = search.results`,
  and positions matching the rendered order

#### Scenario: A serving response without a request id still joins

- **WHEN** the recommendation response for a row carries an empty `request_id`
- **THEN** the impressions and clicks from that row share one `request_id` that starts with `fe-`, and a second
  render of the row uses a different one
