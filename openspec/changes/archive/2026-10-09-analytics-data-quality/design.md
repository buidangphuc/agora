## Context

See proposal.md for the motivation. Current agora code as of 2026-10-09, after `tracking-ingest-integrity`:
- **team-analytics:**
  - The tracking sink (`internal/consumer/tracking.go`) decodes envelopes, batches them and writes them with the
    DuckDB anti-join on `event_id`. A redelivered or re-sent row is skipped silently, and an undecodable message is
    logged and committed.
  - The warehouse has `ingested_at` and the views `tracking_identity` and `tracking_events_resolved`.
  - `AnalyticsQueryService` already serves admin-only RPCs (`GetPlatformOrderSummary`, `ListRecentOrders`) through
    `interceptor.RequireScopes(ctx, "admin")` (`internal/query/service.go:220`).
- **team-gateway:**
  - The cockpit handler (`internal/edge/cockpit.go`) builds the snapshot from Prometheus, those analytics admin RPCs and
    Jaeger.
  - Each upstream failure leaves its field null.
- **team-frontend:** `/admin/cockpit` renders the snapshot from `src/lib/gateway/cockpit.ts`.

## Goals / Non-Goals

**Goals:** one admin-readable report that answers "is tracking healthy right now?", and the same report on the cockpit.

**Non-Goals:** alerting and paging, blocking ingestion, and quality checks on non-tracking facts.

## Decisions

### D1. Counters in the warehouse
- New table `tracking_ingest_counters(hour TIMESTAMP PRIMARY KEY, decode_failures BIGINT, duplicates_skipped
  BIGINT)`, upserted once per written batch (`INSERT ... ON CONFLICT (hour) DO UPDATE SET x = x + excluded.x`).
- Duplicates are the batch size minus the rows actually inserted. The anti-join statement's affected-row count gives
  this per row.
- Decode failures are counted when the consumer drops a message.
- Alternative considered: Prometheus counters. Rejected, because agora has no alerting stack, and the report must work
  from the warehouse alone. Counters in the warehouse also survive restarts.

### D2. The report query
- **Per type:** a single DuckDB query over `tracking_events_resolved` in the window gives `count(*)`,
  `count(DISTINCT user_key)` and `avg(listing_id = '')` for the listing-scoped types.
- **Lag:** `quantile_cont(epoch(ingested_at) - epoch(occurred_at), [0.5, 0.95])` over rows with a non-null
  `ingested_at`.
- **Freshness:** `max(ingested_at)`.
- **Counters:** the sum of `tracking_ingest_counters` rows whose `hour` falls in the window.
- **Status** is computed in Go from the thresholds. `reasons` is a sorted list of strings: `stale`, `lagging`,
  `incomplete`.
- The query reuses the service's in-process DuckDB handle. The writer and the query layer share the process, so there
  is no lock conflict.

### D3. Proto (additive)
```proto
rpc GetTrackingQualityReport(GetTrackingQualityReportRequest) returns (GetTrackingQualityReportResponse);
message GetTrackingQualityReportRequest { uint32 window_hours = 1; }   // 0 = 24
message TrackingTypeQuality { string event_type = 1; int64 events = 2; int64 visitors = 3;
  double missing_listing_ratio = 4; bool listing_scoped = 5; }
message GetTrackingQualityReportResponse {
  repeated TrackingTypeQuality types = 1; double lag_p50_seconds = 2; double lag_p95_seconds = 3;
  google.protobuf.Timestamp last_ingested_at = 4; int64 decode_failures = 5; int64 duplicates_skipped = 6;
  string status = 7; repeated string reasons = 8; uint32 window_hours = 9; }
```
- The new proto is vendored into team-analytics, team-gateway and team-frontend, which are the only `analytics.proto`
  consumers.

### D4. Edge and cockpit
- **Gateway:**
  - The forwarder gains the RPC, with `callRead`.
  - `adminProcedures` gains the procedure.
  - `Snapshot` gains `TrackingQuality *TrackingQuality` (`json:"tracking_quality"`), filled by one call with a
    24-hour window. It stays null on error, like the order fields.
- **Frontend:**
  - A `TrackingQualityPanel` server component in the cockpit page.
  - It uses the existing Card, Badge and Descriptions primitives.
  - It renders the status badge, the reasons, the last ingest time and a per-type table.
  - A null section renders the shared unavailable state.

### D5. E2E
- The report is read through the gateway as the seeded admin.
- The panel is read in the browser.
- The "analytics down" scenario stops team-analytics, so it runs in the destructive lane and restarts it afterwards.
- The incomplete scenario posts its own views without `listingId`. It asserts that the ratio is above 0 and that the
  status follows the configured threshold. It does not assert that the stack-wide ratio exceeds the threshold, because
  other tests post listing views too.

## Risks / Trade-offs

- **The report scans the window of `tracking_events_resolved` on every cockpit refresh.**
  - Mitigation: at local scale this takes milliseconds.
  - Mitigation: the cockpit already refreshes slowly.
  - At larger scale, materialise hourly aggregates. That is a follow-up, and it fits naturally with
    `featurestore-materialization`.
- **The duplicate count depends on the anti-join's affected-row count.** A driver that reports none would count 0.
  - Mitigation: a unit test pins the behaviour with go-duckdb.

## Migration Plan

- The deployment order is platform-core proto and vendoring, then team-analytics, then team-gateway, then
  team-frontend. An old gateway simply never calls the new RPC.
- Rollback is a revert of the images. The table is harmless.
