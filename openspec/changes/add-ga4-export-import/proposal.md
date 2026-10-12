# Proposal: GA4 export import, attributed funnel, reconciliation alert

initiative: ga4-analytics

## Why
GA4 holds the acquisition data (source/medium/campaign) that first-party tracking lacks, while first-party
holds what GA4 loses (adblock, consent, per-slot impressions). Importing the GA4 export into the warehouse lets
ops split funnels by channel and lets us detect when the two pipelines drift apart. Depends on
`ga4-consent-contract` (`TrackingEvent.ga_client_id`, persisted in `tracking_events`).

## What changes
- New job `team-analytics/internal/ga4import` (run by a daily cmd) reads GA4 export `events_YYYYMMDD`
  (BigQuery in prod; Parquet fixture with the GA4 export schema when `GA4_EXPORT_SOURCE=fixture`) and writes
  `ga4_sessions(export_date, user_pseudo_id, session_id, source, medium, campaign, landing_page, ts, ended_at)`; idempotent per export date.
- `warehouse.Schema`-style canonical `GA4SessionsSchema` with DuckDB and BigQuery adapters and a parity test
  (migrations use `ADD COLUMN IF NOT EXISTS`); `ga4_events_daily` counts table for reconciliation.
- View `tracking_events_attributed`: `tracking_events_resolved` joined to `ga4_sessions` on
  `ga_client_id = user_pseudo_id` (`ga_client_id` is stored without the `GA1.1.` prefix, the same form as GA4 `user_pseudo_id`).
- Funnel split by source/medium/campaign for ops: query layer on the attributed view, served by the new admin-only RPC `AnalyticsQueryService.GetChannelFunnel` (request: `from`, `to` (Timestamp) + optional `seller_id`; response `repeated ChannelFunnelRow rows` of `{source, medium, campaign, impressions, views, adds, begin_checkouts, purchases}` int64), whose proto is provided by slice `ga4-consent-contract`.
- Reconciliation in `tracking-data-quality`: daily GA4 vs first-party event counts beyond a configured
  threshold mark the quality report DEGRADED (reason `ga4_divergence`) and raise a Prometheus alert.
- `team-gateway`: edge policy entry `AnalyticsQueryServiceGetChannelFunnelProcedure: adminScope` in `internal/edge/policy.go` (after vendoring/regenerating the `analyticsv1connect` stub from `ga4-consent-contract`).
- `platform-gitops`: CronJob + config (`GA4_EXPORT_*`, `GA4_RECONCILE_MAX_DIFF_RATIO`) and an alert rule.
- `export.go` adds `ga4_sessions` / `tracking_events_attributed` to the Parquet targets (hot file, serialised with
  `recsys-consent-and-context`).

## Non-goals
- GA4 is never a revenue source: no query SUMs revenue from GA4; revenue stays on `order_facts`.
- GA4 is not a recsys training source.
- No GA4 import in dev/CI/e2e against Google; the fixture is used.

## Preconditions
- `GetTrackingQualityReport` is already admin-only: `team-analytics/README.md` (scope table), the main spec `openspec/specs/tracking-data-quality/spec.md` (anonymous 401 / buyer 403 scenario) and `openspec/specs/edge-route-policy/spec.md` gate it at interceptor and gateway. This change relies on that gate and adds reconciliation reasons behind it; it does not change the gate.
- `AnalyticsQueryService.GetChannelFunnel` exists in the proto (slice `ga4-consent-contract`).

## NFR
(Testable ones have scenarios: import duration, funnel latency. The rest are operating notes.)
- Runs once daily after the export lands; one run for one export date completes in under 10 min for 5 M GA4 rows.
- Idempotent per export date: re-running the same date leaves `ga4_sessions` row count and content unchanged.
- Failure of the import never blocks tracking ingest or the existing quality report.
- Attributed funnel query p95 < 2 s over a 30-day window.

## Data classification
| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `ga4_sessions.user_pseudo_id` (GA client id) | pseudonymous identifier | join to first-party events for attribution | analytics consent (GA4 only holds it when granted) | same as `tracking_events` (13 months raw) |
| `session_id`, `source`, `medium`, `campaign`, `landing_page`, `ts` | behavioural, non-identifying | channel attribution | analytics consent | same as above |
- No email, phone or name is read or stored; `landing_page` query strings are reduced to scheme/host/path
  (no query string) at import, like the gateway referrer scrub.
- Data-subject deletion: rows keyed by `user_pseudo_id` are deletable with the same procedure as `tracking_events`.

## Threat model
- Assets: `ga4_sessions`, attributed view, alert config.
- Abuse: (1) forged fixture/export giving fake channel data; mitigated by service-account read-only access to the
  export dataset and no inbound endpoint. (2) Seller reads other sellers' or platform channel data; funnel by
  channel is `admin` scope only. (3) Re-run or backfill double-counting; mitigated by delete-then-insert per export date in one transaction.
  (4) PII in `landing_page`; scrubbed at import.
- Authz: channel funnel and reconciliation figures require the `admin` scope, enforced by the interceptor and gated at the gateway.
