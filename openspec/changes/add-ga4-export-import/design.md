# Design

- Session identity and window: `ga4_sessions` has `export_date` (the export day D the row came from) so replace is `DELETE ... WHERE export_date = D` then insert. A session has `ts` (first GA4 event) and `ended_at` (last GA4 event ts of that session), both written by the import. Session window = [`ts`, min(next session `ts` of the same `user_pseudo_id`, `ended_at` + 30 min)). An event is attributed to the session whose window contains its time (windows of one client do not overlap, so no row multiplication); otherwise `source`='(none)'.
- Join key: `tracking_events.ga_client_id` holds the client-id part only (`111.222`, no `GA1.1.` prefix), equal to GA4 `user_pseudo_id`.
- Import: per export date D, read `events_D` (`session_start` and `first_visit` + `page_view` events), derive one row per
  (`user_pseudo_id`, `ga_session_id`) with `source/medium/campaign` from `traffic_source` (or event params
  `source`,`medium`,`campaign`), `landing_page` = first `page_location` (query stripped), `ts` = session start.
  Write in one transaction: `DELETE WHERE export_date = D` then insert. A `ga4_import_runs(export_date, rows, finished_at)` row is written last.
- `ga4_events_daily(day, event_name, events)` is also filled from the same read for reconciliation; compared against
  first-party `tracking_events` of the same UTC day for event types that GA4 also receives (view, click, add-to-cart, begin_checkout; `purchase` excluded because the server relay owns it).
- Divergence ratio = |ga4 - first_party| / max(first_party, 1). Threshold `GA4_RECONCILE_MAX_DIFF_RATIO` (default 0.35: GA4 normally loses 10-30%).
  Days with no import run are `ga4_missing`, not divergence; `ga4_missing` adds a reason only and leaves `status` unchanged (default, flagged for LOCK).
- Surfaces: `GetTrackingQualityReport.reasons` gains `ga4_divergence` / `ga4_missing` (string, no proto change); gauge
  `ga4_reconcile_diff_ratio{event_type}` drives a Prometheus alert in platform-gitops. The channel funnel is served by `AnalyticsQueryService.GetChannelFunnel` (admin-only; request `from`, `to` (Timestamp) + optional `seller_id`; `repeated ChannelFunnelRow` `{source, medium, campaign, impressions, views, adds, begin_checkouts, purchases}` int64), proto owned by `ga4-consent-contract`; this slice implements the handler on `tracking_events_attributed`.
- Parity: `GA4SessionsSchema` and `GA4EventsDailySchema` live beside `Schema`; adapters derive DDL from them.
- Gateway: `team-gateway/internal/edge/policy.go` maps `AnalyticsQueryServiceGetChannelFunnelProcedure` to `adminScope` once the `analyticsv1connect` stub is regenerated from `ga4-consent-contract`.
