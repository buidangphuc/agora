## Why

AI-first change 2 of 7. `tracking-ingest-integrity` made rows clean, idempotent and timestamped (`ingested_at`), but
nobody can see whether the tracking stream is healthy:
- whether events stopped arriving;
- whether they arrive late;
- whether listing-scoped events lost their `listing_id`;
- how many re-sends and undecodable messages the sink absorbed.

The feature store and the trainer (changes 4–6) will read this data. A silent outage or a broken tracker would train
models on a hole, and nothing would say so. The user decided on 2026-10-08 that data quality comes now, while
erasure waits for legal.

## What Changes

- **platform-core (proto, additive):** `AnalyticsQueryService.GetTrackingQualityReport` (admin-only), with its request
  and response messages.
- **team-analytics:**
  - The tracking sink counts undecodable messages and duplicate re-sends per hour in `tracking_ingest_counters`.
  - The new RPC computes these from the warehouse over a window:
    - per event type: count, distinct visitors and missing-`listing_id` ratio;
    - ingest lag p50/p95;
    - the last ingest time;
    - the counters.
  - It derives a status: `OK`, or `DEGRADED` with `stale`, `lagging` and/or `incomplete` reasons.
- **team-gateway:**
  - Route the RPC, admin-gated in `adminProcedures`.
  - The cockpit snapshot (`/api/admin/metrics`) gains a `tracking_quality` section built from it.
- **team-frontend:** `/admin/cockpit` shows a "Tracking data quality" panel: the status, the reasons, the last ingest
  time and per-type counts. It shows an unavailable state when the section is null.
- **Re-vendor** `analytics.proto` into team-analytics, team-gateway and team-frontend.
- **platform-e2e:** scenarios through the edge and the cockpit.

Repos touched: platform-core, team-analytics, team-gateway, team-frontend, platform-e2e. **Proto change: additive.**

## Capabilities

### New Capabilities
- `tracking-data-quality`: the ingest counters, the quality report, its thresholds and status, who may read it, and how
  the cockpit shows it.

### Modified Capabilities
- `edge-route-policy`: the admin-gated set gains `AnalyticsQueryService/GetTrackingQualityReport`.

## Non-goals

- Prometheus alert rules and paging. agora ships no alerting stack today. The report's status is the signal, and a
  rule can be added when alerting lands.
- Erasure, retention and consent (legal deferral).
- Quality checks on engagement or order facts (`engagement-fact-events` is change 3).
- Blocking ingestion on bad quality. The report observes; it never drops data.

## Impact

- **New team-analytics settings:**
  - `TRACKING_STALE_AFTER_SECONDS` (default 900)
  - `TRACKING_LAG_P95_MAX_SECONDS` (default 300)
  - `TRACKING_MISSING_LISTING_MAX_RATIO` (default 0.05)
- **Warehouse:** a new table `tracking_ingest_counters`, created idempotently.
- **Cockpit JSON:** a new nullable field. Existing fields are unchanged.
