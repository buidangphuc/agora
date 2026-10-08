# Hand-off: tracking-event-platform (C1) -> analytics-feature-store (C2)

OpenSpec task: `tracking-event-platform` 12.3. Source of truth is the C1 design
(`openspec/changes/tracking-event-platform/design.md`, D7/D8/D10/D11/D13) and team-analytics `internal/warehouse`.
C1 owns the tracking tables and retention; C2 reads them and must not write to them.

## Tables (DuckDB `analytics.duckdb`, DDL from `internal/warehouse` `[]Column` lists)

- `tracking_events`: wide client-event table, PK `event_id` (insert is `ON CONFLICT DO NOTHING`, a resent beacon is stored once).
  Columns include the 11 client types' attribution fields (`placement_id, request_id, model_version, dwell_ms, result_count,
  filters, client_event_id, client_occurred_at, quantity, share_channel, sample_rate, page_type, listing_ids, consent_state,
  schema_version`).
- Fact tables (typed business records, not behaviour): `order_facts`, `order_fact_items`, `order_status_changes`, `favorite_facts`,
  `review_facts`, `follow_facts`, `payment_facts`.
- Support tables: `identity_links`, `ingest_quarantine`, `ingest_watermarks`, `erasure_log`, `erasure_tombstones`,
  `retention_runs`, `dq_runs`, `dq_results`.
- Views (re-created on start): `tracking_events_resolved`, `consent_state_latest`.

## Columns and views C2 depends on

- `ingested_at` (TIMESTAMP, microsecond precision): stamped by the writer inside `warehouse.IngestGate.Write`; it is C2's
  visibility clock. `IngestGate.Fence(T)` guarantees the set `ingested_at <= T` is complete and immutable. Never use
  `occurred_at` as the as-of for reproducible datasets.
- `tracking_events_resolved` = `tracking_events` + `subject_id`: user principal -> user id; else the single user the
  `anonymous_id` links to; else `anon:<anonymous_id>` (a shared device stays anonymous). Retroactive stitching: fine for
  serving/cold-start reads, WRONG for point-in-time training.
- `identity_links(anonymous_id, user_id, first_seen_at, last_seen_at, identify_count)`: the point-in-time builder must join on
  `first_seen_at <= as_of` instead of using the view, otherwise a future login leaks into a past training row (data leakage).
- `consent_state_latest(subject_key, consent_state, occurred_at, ingested_at)`: latest CONSENT_UPDATE per subject
  (`subject_key` = user id, or `anon:<anonymous_id>`). Opted-out (`denied`) subjects must be excluded from personalization
  features and datasets. Server-side facts keep flowing (business records).

## Erasure (C1 6.3 <-> C2 8.4)

- `EraseSubject(user_id)` (admin RPC via gateway) deletes raw rows + identity links in one DuckDB transaction, pseudonymises
  fact rows (`erased:<hmac>`), inserts `erasure_tombstones(hmac(user_id) and each linked hmac(anonymous_id))`, logs to
  `erasure_log`, then calls `ops.ErasureHook.AfterErase(ctx, userID, anonymousIDs)`. C2 registers its hook
  (`internal/features/erasure`): online keys, offline partition rewrites, dataset invalidation. The hook must be idempotent
  (a failed hook is retried by calling EraseSubject again).
- The consumer checks tombstones before insert, so replays cannot restore erased rows. C2 datasets MUST filter tombstoned
  subjects. Kafka retention bounds raw residue (analytics.events 7 d, order/engagement.events 14 d).
- `FS_ERASURE_REWRITE_NOW` (test/local only, refused in a strict ENV) makes the erase wait for the partition rewrite.

## Retention (decision U2, legal review pending)

- C1's `internal/retention` job is the SINGLE owner of raw-event retention: free text nulled after 90 d; behavioural rows
  13 months; identity links 13 months after last seen; facts 25 months; quarantine 14 d; DQ runs 13 months. C2's sweep only
  manages its own artifacts (offline partitions 180 d, datasets 30 d) and MUST NOT delete raw rows.
- The job is OFF by default and OFF in gitops (`RETENTION_ENABLED` absent) until legal signs off; consent mode stays `opt_out`.
  Do not assume retention is running when sizing the PVC.

## Freshness and quality inputs

- Watermarks: `ingest_watermarks(table_name, watermark, updated_at)`; value = `max(occurred_at seen) - ANALYTICS_LATE_WINDOW_HOURS`
  (72 h), monotonic, exported as `analytics_ingest_watermark_seconds{table}`. A healthy table therefore sits about one late
  window behind the newest row; do not treat that as ingestion lag.
- The dataset ingestion watermark (distinct from the table watermark) is `min(now, max ingested_at)` and keeps microsecond
  precision since team-analytics@04741eb. Before it, flooring to the second excluded the newest sub-second batch from every
  `as_of = watermark` snapshot (`ingested_at <= as_of`) and the recsys holdout evaluated 0 users. Keep full precision end to end;
  `fmtTime` emits whole seconds as plain RFC 3339 and sub-second times with exactly six digits.
- Data quality: `dq.Engine.Report(ctx, table, eventType)` is the in-process hook (`quality.Reporter`); the feature store runs in
  the same process (single replica), and `quality.Gate.c1` fails a source whose table verdict is `fail` (verdicts older than
  `MaxVerdictAge`, a nil reporter or a report error pass). The same verdict is available externally as `GetDataQualityReport`
  (admin scope only) and `analytics_dq_status{table,event_type,check}` (0 pass, 1 warn, 2 fail, 3 insufficient_baseline;
  run every `DQ_INTERVAL`=15 m).
- Alerts: `AnalyticsDataQualityFailing`, `AnalyticsIngestLagHigh`, `GatewayTrackDropRateHigh`, `OutboxRelayLagHigh`
  (platform-core `infra/observability/prometheus/rules/`).

## Not C2's to change

`taxonomy.v1.yaml` and `redaction-vectors.json` (vendored copies are drift-checked by `scripts/repo_doctor.py`), the gateway
consent mode, `RETENTION_*` windows.
