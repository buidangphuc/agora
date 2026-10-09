## Why

This is AI-first change 8 of 8, the last one. Offline holdout metrics now gate promotion (changes 5–6), but nothing
measures what a model does in production. Per placement and per model version, nobody can see:
- how often a recommended item was clicked;
- how often it led to an add-to-cart or a purchase;
- how often serving fell back.

`recs-serving-safeguards` made every recommendation response carry a server `request_id`, and the storefront sends it as
the beacons' `impressionId`. The warehouse can therefore join impressions, clicks and later purchases to the exact list
that was served.

## What Changes

- **platform-core (proto, additive):** `AnalyticsQueryService.GetRecommendationPerformance`, admin-only, with its
  messages. Vendored into team-analytics, team-gateway and team-frontend.
- **team-analytics:** the RPC computes, over a window (1–168 hours), for each (placement, model_version):
  - impressions: distinct impression ids;
  - item impressions;
  - clicks;
  - add-to-carts and purchases attributed within `RECS_ATTRIBUTION_WINDOW_HOURS` (default 24) of a click from that
    impression;
  - CTR and conversion rate;
  - the fallback share (impressions served with `model_version` `serving-fallback`).
- **team-gateway:** routes the RPC and gates it in `adminProcedures`.
- **platform-e2e:** scenarios through the edge.

Repos touched: platform-core, team-analytics, team-gateway, team-frontend (re-vendor only), platform-e2e. **Proto change:
additive.**

## Capabilities

### New Capabilities
- `recsys-online-evaluation`: how recommendation outcomes are attributed and reported per placement and model.

### Modified Capabilities
- `edge-route-policy`: the admin-gated set gains `AnalyticsQueryService/GetRecommendationPerformance`.

## Non-goals

- A/B assignment or significance testing. Comparison is descriptive: each row is one served model.
- Automatic rollback on bad online metrics. The `rollback` command from change 6 stays an operator action.
- A cockpit panel. The RPC is enough for now; a panel can follow the pattern of the tracking-quality panel.

## Impact

- New team-analytics setting: `RECS_ATTRIBUTION_WINDOW_HOURS`.
- No new table: the report reads `tracking_events_resolved`.
