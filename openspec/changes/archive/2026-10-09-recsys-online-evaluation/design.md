## Context

See proposal.md for the motivation.

**Current state** (2026-10-09, after `recs-serving-safeguards`):
- The storefront sends recommendation rows' beacons with `impressionId` = the server `request_id`, `placementId` = the
  served placement, and `modelVersion`.
- `tracking_events_resolved` holds `event_type`, `listing_id`, `user_key`, `placement_id`, `impression_id`,
  `model_version` and `occurred_at`.
- The admin RPC pattern is `GetTrackingQualityReport`:
  - `RequireScopes(admin)` in `internal/query/service.go`;
  - the query lives in `DuckDBRepository` behind an optional interface;
  - the gateway forwarder, plus `adminProcedures`.

## Goals / Non-Goals

**Goal:** a descriptive per-(placement, model) report of served lists and their outcomes.

**Non-goals:** A/B assignment, statistics, automatic actions, and UI.

## Decisions

### D1. Attribution in SQL over the resolved view
- `imp`: impression events in the window that have a non-empty `impression_id` and `placement_id`.
- `clk`: click events whose `impression_id` is in `imp`. Each carries its `user_key`, `listing_id` and `occurred_at`.
- `conv`: add-to-cart and purchase events joined to `clk` on (`user_key`, `listing_id`), with
  `occurred_at BETWEEN clk.occurred_at AND clk.occurred_at + INTERVAL window HOUR`.
  - Each conversion event is counted once, at its earliest attributable click.
  - It goes to that click's placement and model.
- Results are grouped by (`placement_id`, `model_version`).
- The fallback share per placement is the impressions with `model_version='serving-fallback'` divided by all of that
  placement's impressions.

### D2. Proto (additive)
```proto
rpc GetRecommendationPerformance(GetRecommendationPerformanceRequest) returns (GetRecommendationPerformanceResponse);
message GetRecommendationPerformanceRequest { uint32 window_hours = 1; }
message RecommendationPerformanceRow { string placement_id = 1; string model_version = 2; int64 impressions = 3;
  int64 item_impressions = 4; int64 clicks = 5; int64 add_to_carts = 6; int64 purchases = 7; double ctr = 8;
  double conversion_rate = 9; }
message PlacementFallbackShare { string placement_id = 1; double fallback_share = 2; }
message GetRecommendationPerformanceResponse { repeated RecommendationPerformanceRow rows = 1;
  repeated PlacementFallbackShare fallback = 2; uint32 window_hours = 3; uint32 attribution_window_hours = 4; }
```
The file is vendored into team-analytics, team-gateway and team-frontend, the same three as before.

### D3. Edge
- A forwarder method using `callRead`.
- An `adminProcedures` entry, with the pin test updated.

## Risks / Trade-offs

- **[Click-to-purchase attribution over-credits when a listing is clicked in two rows]** Mitigation: each conversion is
  credited to its earliest attributable click only.
- **[Event volume]** Mitigation: the windowed query over the resolved view is fine at local scale. The same
  materialisation note as the quality report applies.

- **[Attribution is client-reported, so it is forgeable]**
  - Impressions, clicks and purchase beacons all come from the browser.
  - Mitigation: the report is descriptive and admin-only. It drives no payout and no automatic rollback.
  - A follow-up can source purchases from the server-truth order facts once those carry a buyer.
- **[Visitors with no anonymous id share the key `anon:`]**
  - Mitigation: conversions on that key are not attributed.

## Migration Plan

Deploy in this order: proto and vendoring, then team-analytics, then team-gateway. The change is additive.
