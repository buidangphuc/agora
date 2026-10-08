## ADDED Requirements

### Requirement: The tracking sink counts what it absorbs

The team-analytics tracking sink SHALL record, per UTC hour, the number of messages it could not decode and the number of
events it skipped because a row with the same `event_id` already existed, in `tracking_ingest_counters`
(`hour`, `decode_failures`, `duplicates_skipped`). A counter write failure SHALL be logged and SHALL NOT block ingestion.

#### Scenario: A re-sent event is counted as a skipped duplicate

- **WHEN** a visitor posts the same view (same `eventId` and `anonymousId`) twice and the admin then reads the tracking
  quality report for the last hour
- **THEN** the report's `duplicates_skipped` is at least 1 higher than it was before the two posts

### Requirement: Admins can read a tracking quality report

`AnalyticsQueryService/GetTrackingQualityReport` SHALL take a window of 1 to 168 hours (default 24). It SHALL return:
- per event type: the event count, the number of distinct `user_key`s from `tracking_events_resolved`, and, for view,
  click, add-to-cart and impression, the ratio of events with an empty `listing_id`;
- the p50 and p95 of `ingested_at - occurred_at` in seconds;
- the latest `ingested_at`;
- the summed `decode_failures` and `duplicates_skipped`;
- a status of `OK`, or `DEGRADED` with one or more reasons:
  - `stale` when the latest `ingested_at` is older than `TRACKING_STALE_AFTER_SECONDS`, or no event exists in the
    window;
  - `lagging` when the p95 lag exceeds `TRACKING_LAG_P95_MAX_SECONDS`;
  - `incomplete` when any listing-scoped ratio exceeds `TRACKING_MISSING_LISTING_MAX_RATIO`.

Rows with a null `ingested_at` (written before `tracking-ingest-integrity`) SHALL be excluded from the lag statistics. A
window outside 1–168 hours SHALL fail with `INVALID_ARGUMENT`. The RPC SHALL require the `admin` scope; the gateway SHALL
gate it at the edge.

#### Scenario: Fresh views appear in the report

- **WHEN** a visitor posts three views of a listing and the admin reads the report for the last hour
- **THEN** the view count is at least 3, the latest ingest time is within the last 5 minutes, and the status is not
  degraded for `stale`

#### Scenario: Views without a listing make the report incomplete

- **WHEN** a visitor posts 50 views without a `listingId` and the admin reads the report for the last hour
- **THEN** the view type's missing-listing ratio is above 0 and, if it exceeds the configured maximum, the status is
  `DEGRADED` with reason `incomplete`

#### Scenario: A window out of range is rejected

- **WHEN** the admin reads the report with a window of 500 hours
- **THEN** the call fails with `invalid_argument`

#### Scenario: Only admins can read the report

- **WHEN** an anonymous client and then a logged-in buyer call `GetTrackingQualityReport` through the gateway
- **THEN** the gateway answers HTTP 401 and then HTTP 403

### Requirement: The cockpit shows tracking data quality

The cockpit snapshot (`GET /api/admin/metrics`) SHALL carry a `tracking_quality` section with the report for the last
24 hours, or null when team-analytics cannot be reached. `/admin/cockpit` SHALL show a "Tracking data quality" panel with
the status, any reasons, the latest ingest time and the per-type counts. When the section is null, the panel SHALL say
the data is unavailable rather than show zeros.

#### Scenario: The admin sees the tracking quality panel

- **WHEN** the seeded admin opens `/admin/cockpit` after views were tracked
- **THEN** the "Tracking data quality" panel shows a status and a view count of at least 1

#### Scenario: The panel says unavailable when analytics is down

- **WHEN** team-analytics is stopped and the admin opens `/admin/cockpit`
- **THEN** the "Tracking data quality" panel says the data is unavailable and shows no counts
