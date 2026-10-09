## Context

See proposal.md for the motivation. Current agora code as of 2026-10-08:
- `team-gateway/internal/edge/collector.go`:
  - `HandleTrack` validates the whole batch before producing anything, and rejects the batch on the first unknown type.
  - `parseBeacons` accepts one object or an array of at most 100.
  - The publisher (`internal/events/publisher.go:78`) mints `EventId: uuid.NewString()` and stamps
    `OccurredAt: timestamppb.Now()`.
  - Since `port-edge-authz-residuals`, the route sits behind `edgeHTTP` (request id, `TRACK_RATE_LIMIT_*`, log line).
- `team-analytics/internal/warehouse`:
  - The `Schema` is a column list; `ensureSchema` adds missing columns idempotently.
  - The DuckDB insert is an anti-join on `event_id` (`duckdb.go:149-163`).
  - BigQuery uses the same `Schema`.
- `team-frontend/src/lib/analytics/{dispatcher,queue}.ts`:
  - `bds_anonymous_id` lives in localStorage and the session id in sessionStorage.
  - The queue flushes every 2 s or at 20 events with `sendBeacon`, falling back to `fetch` with `keepalive`.

## Goals / Non-Goals

**Goals:** clean, idempotent and stitchable tracking rows with no proto change. Every rule is observable on
`analytics.events` or in the warehouse file.

**Non-Goals:** consent and retention (legal deferral), new event types, data-quality metrics (next change).

## Decisions

### D1. Validation per event, in the gateway
- A `validateBeacon(b) error` function applies the type and bounds rules.
- The handler partitions the batch into accepted and dropped events, publishes the accepted ones, and answers 202
  with the counts. A body with zero accepted events answers 400 with a short reason.
- Bounds live as constants beside `maxBatchItems`.
- Rule 2 holds: this is input validation at the edge, with no business logic.

### D2. Deterministic `event_id`
- `event_id = uuid5(NS_TRACK, visitorKey + "|" + eventId)`.
- `visitorKey` is the verified principal id when the beacon carries a valid token or session cookie. Otherwise it is
  `anon:` plus the beacon's `anonymousId`. When neither is present the event keeps a random id.
- `NS_TRACK` is a fixed UUID constant in the gateway.
- The publisher gains an optional event id argument (`PublishTrackingEvent(ctx, ev, principal, requestID, eventID)`),
  and an empty value keeps `uuid.NewString()`.
- Scoping by visitor stops one visitor from suppressing another's events by reusing an `eventId`.
- The warehouse anti-join already dedupes on `event_id`, so the sink needs no change for this.

### D3. Scrubbing in the gateway, before Kafka
- One `scrubText` function with the three regexes:
  - email: `[\w.+-]+@[\w-]+(\.[\w-]+)+`
  - VN phone: `(?:\+84|\b0)(?:[\s.]?\d){9}\b`
  - long digit run: `\d{13,}`
- It is applied to `search_query`, `page_path` and the `properties` values.
- `referrer` is parsed with `net/url` and rebuilt as `scheme://host/path`. An unparseable referrer becomes empty.
- Scrubbing happens before Kafka so that personal data never lands on the topic or in replays.
- Alternative considered: scrub in team-analytics. Rejected, because the raw text would still sit in Kafka for its
  retention period and in any other consumer.

### D4. `ingested_at`
- A new `Schema` column `{"ingested_at", "TIMESTAMP", "TIMESTAMP"}`, set by the writer to `time.Now().UTC()` per batch
  row.
- DuckDB gets it through the existing `ALTER ... ADD COLUMN IF NOT EXISTS`. The BigQuery schema list gains it too.
- `occurred_at` is unchanged.

### D5. Stitching views (DuckDB)
`ensureSchema` runs `CREATE OR REPLACE VIEW` for the two views:

```sql
CREATE OR REPLACE VIEW tracking_identity AS
SELECT anonymous_id, any_value(principal_id) AS principal_id
FROM tracking_events
WHERE principal_type = 'user' AND anonymous_id <> ''
GROUP BY anonymous_id
HAVING count(DISTINCT principal_id) = 1;   -- ambiguous ids are not stitched (review, 2026-10-09)

CREATE OR REPLACE VIEW tracking_events_resolved AS
SELECT t.*, CASE
  WHEN t.principal_type = 'user' THEN t.principal_id
  WHEN i.principal_id IS NOT NULL THEN i.principal_id
  ELSE 'anon:' || t.anonymous_id END AS user_key
FROM tracking_events t LEFT JOIN tracking_identity i USING (anonymous_id);
```

- The exact `principal_type` literal must match what the sink stores; the code track checks this.
- BigQuery gets equivalent DDL in `warehouse/bigquery` (`ANY_VALUE ... HAVING MAX`), created on start when the dataset
  is writable, and otherwise documented as a migration.

### D6. Frontend `eventId` and one retry
- `trackEcommerce` stamps `eventId: randomId()` per beacon, including per item of a fan-out.
- `WireTrackBeacon` gains `eventId`.
- On the `fetch` fallback, a network error re-sends once after 1 s with the same body. A `sendBeacon` that returns
  false falls back to `fetch` as today.

### D7. E2E observation
- Kafka assertions reuse `tests/e2e/flows/tracking_flow.py` (`consume_tracking_events`).
- Warehouse assertions query the DuckDB file read-only. DuckDB allows only one writer, so the steps read it with
  `docker exec agora-team-analytics-svc` and a duckdb CLI or Python one-liner when available. Otherwise they copy the
  file out (`docker cp`) and open it read-only with the e2e venv's `duckdb` package.
- The code track documents which one the image supports, and the e2e track uses it.

## Risks / Trade-offs

- **Clients that relied on 204** → `sendBeacon` ignores the body, and the frontend treats any 2xx as success. Other
  clients are e2e only.
- **The phone regex could mask a 10-digit product code starting with 0** → This is an acceptable privacy-first bias.
  Prices rarely start with 0, and the scenarios pin which numbers are kept.
- **A copy of the DuckDB file in e2e can be read mid-write** → Steps poll until the row appears, and the copy is
  read-only.
- **The views cost a full scan on large warehouses** → They are local and dev scale. BigQuery would materialise them
  later if needed (featurestore-materialization).

## Migration Plan

- The order of deployment does not matter:
  - The old frontend sends no `eventId`, so it gets random ids as today.
  - The new frontend against the old gateway has its `eventId` ignored, because unknown JSON fields are dropped.
- Existing warehouse rows get a null `ingested_at`.
- Rollback is a revert of the images. The added column and views are harmless.
