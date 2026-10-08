## Purpose

Defines how team-analytics turns the event streams into warehouse tables that downstream consumers (the feature
store, the recommender export, seller dashboards) can trust: layout, exactly-once rows under at-least-once delivery,
late arrivals, identity stitching, an ingestion watermark, and an admin inspection path.

## ADDED Requirements

### Requirement: Behavioural events land in one wide table and facts in typed tables

The warehouse SHALL store every client tracking event in the wide behavioural table `tracking_events` (existing
columns kept, new columns appended, one row per event, every taxonomy field a column) and SHALL store server facts in
typed tables (orders with line items, order status changes, favorites, reviews, follows, payments). Both the DuckDB
and BigQuery adapters SHALL derive their schemas from the same column lists.

#### Scenario: Every client event type lands with its shape

- **WHEN** one event of each of the 11 client types is collected for one visitor
- **THEN** `tracking_events` holds 11 rows whose `event_type` values are the 11 names and whose type-specific
  columns (for example `dwell_ms` for `view`, `result_count` for `search`, `quantity` for `add_to_cart`,
  `share_channel` for `share`) are populated

#### Scenario: Existing readers keep working

- **WHEN** the recommender export queries `tracking_events` by its pre-existing column names after the migration
- **THEN** the query succeeds and returns the same values for rows written before the migration

### Requirement: Rows are unique per event id

The warehouse SHALL hold at most one row per `event_id` in every table, regardless of producer retries, consumer
redelivery or replay of a topic from the earliest offset. The gateway SHALL derive the envelope `event_id` from the
client's `client_event_id` when it is a valid UUID so a re-sent beacon deduplicates too.

#### Scenario: A resent beacon is stored once

- **WHEN** the same batch is posted twice because the browser retried
- **THEN** each event appears exactly once in `tracking_events`

#### Scenario: Replaying a topic does not duplicate rows

- **WHEN** the consumer group is reset to earliest and the topic is consumed again
- **THEN** row counts per table are unchanged

### Requirement: Late events are accepted inside a window and quarantined outside it

The consumer SHALL store an event whose `occurred_at` is within the late-arrival window (default 72 hours) behind the
time it is consumed, and SHALL write an event outside the window, or one it cannot decode, to a quarantine table with
a reason instead of the main tables, without stalling the partition. The consumer SHALL publish, per table, an
ingestion watermark (the time up to which data is considered complete: newest consumed event time minus the late
window, never moving backwards).

#### Scenario: A late event inside the window is kept

- **WHEN** an event that occurred 10 hours ago is consumed now
- **THEN** it is stored in `tracking_events` with its original `occurred_at`

#### Scenario: A malformed record is quarantined, not fatal

- **WHEN** a record on `analytics.events` cannot be decoded
- **THEN** it is written to the quarantine table with reason `decode_error`, the offset advances, and the next record
  is processed

#### Scenario: The watermark never moves backwards

- **WHEN** a late event older than the current watermark is stored
- **THEN** the published watermark for that table is unchanged

### Requirement: Anonymous and authenticated activity are stitched

The warehouse SHALL maintain an identity link table fed only by `IDENTIFY` events whose envelope principal is an
authenticated user, linking `anonymous_id` to `user_id` with first and last seen times, and SHALL expose a resolved
subject for each event: the user id when the principal is a user or when the event's `anonymous_id` is linked to
exactly one user, otherwise `anon:<anonymous_id>`. An `anonymous_id` linked to more than one user SHALL resolve
pre-login events to `anon:<anonymous_id>` (shared device).

#### Scenario: An identify from an anonymous principal is ignored

- **WHEN** an `IDENTIFY` event arrives with an anonymous envelope principal
- **THEN** no identity link is created

#### Scenario: A shared device is not stitched

- **WHEN** one `anonymous_id` has identified as user A and later as user B
- **THEN** anonymous events from before either login resolve to `anon:<anonymous_id>`, and events with an
  authenticated principal resolve to that principal

### Requirement: The latest consent state per subject is available to downstream consumers

The warehouse SHALL expose, per subject (the user id for an authenticated principal, otherwise
`anon:<anonymous_id>`), the latest consent state recorded by `CONSENT_UPDATE` events, with its occurrence and
ingestion times; an update sent by a logged-in user SHALL also apply to that browser's anonymous subject. The feature
store reads it to exclude opted-out subjects from personalization.

#### Scenario: The latest choice wins

- **WHEN** a logged-in buyer opts out and later opts back in
- **THEN** the consent state for the buyer's user id and for the browser's `anon:` subject is `granted`, with the
  time of the second update

### Requirement: Admins can inspect stored events by attribution and subject

`team-analytics` SHALL expose an admin-only read RPC returning stored rows filtered by `event_id`, `request_id`,
`anonymous_id`, `session_id` or user id (bounded page size), routed through the gateway. Non-admin callers SHALL get
`PERMISSION_DENIED`.

#### Scenario: Impression and click join on request id

- **WHEN** an admin inspects events by a `request_id` from a row the visitor saw and clicked
- **THEN** the result contains the impressions and the click for that request with matching `placement_id`,
  `model_version` and positions

#### Scenario: A buyer cannot inspect events

- **WHEN** a buyer calls the inspection RPC
- **THEN** it answers `PERMISSION_DENIED`
