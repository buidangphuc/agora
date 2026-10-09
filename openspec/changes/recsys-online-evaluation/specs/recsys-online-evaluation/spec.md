## ADDED Requirements

### Requirement: Recommendation outcomes are reported per placement and model

`AnalyticsQueryService/GetRecommendationPerformance` SHALL take a window of 1 to 168 hours (default 24) and return one row per
(`placement_id`, `model_version`) seen on impression events in the window:
- `impressions`: distinct `impression_id`s;
- `item_impressions`: impression events;
- `clicks`: click events carrying one of those impression ids;
- `add_to_carts` and `purchases`: add-to-cart and purchase events by the same `user_key` on a clicked listing, within
  `RECS_ATTRIBUTION_WINDOW_HOURS` after the click;
- `ctr`: clicks over item impressions;
- `conversion_rate`: purchases over clicks (0 when there are no clicks).

Rows whose `model_version` is `serving-fallback` SHALL be reported like any other. The response SHALL also give, per
placement, the share of impressions served by the fallback. A window outside 1–168 hours SHALL fail with
`INVALID_ARGUMENT`. The RPC SHALL require the `admin` scope, and the gateway SHALL gate it at the edge.

#### Scenario: Impressions and clicks are counted per placement and model

- **WHEN** a visitor posts 2 impressions of a recommendation row (one `impressionId`, placement "home_feed", model
  version M) and 1 click on one of its listings with the same `impressionId`
- **THEN** the admin's report for the last hour has a row for "home_feed" and M with at least 1 impression, at least 2
  item impressions and at least 1 click

#### Scenario: A purchase after a recommended click is attributed

- **WHEN** a logged-in buyer clicks a recommended listing (impression id I, placement "similar_items", model version M2)
  and then purchases that listing
- **THEN** the report row for "similar_items" and M2 counts that purchase

#### Scenario: A purchase without a recommended click is not attributed

- **WHEN** a logged-in buyer purchases a listing they never clicked from a recommendation row
- **THEN** no report row's purchase count includes that purchase

#### Scenario: The fallback share is reported

- **WHEN** a visitor posts impressions for placement "cart_cross_sell" with model version "serving-fallback" and with a
  real model version, 1 each
- **THEN** the report gives "cart_cross_sell" a fallback share above 0 and below 1

#### Scenario: Only admins read recommendation performance

- **WHEN** an anonymous client and then a logged-in buyer call `GetRecommendationPerformance` through the gateway
- **THEN** the gateway answers HTTP 401 and then HTTP 403
