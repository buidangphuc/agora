## MODIFIED Requirements

### Requirement: Recommendation outcomes are reported per placement and model

`AnalyticsQueryService/GetRecommendationPerformance` SHALL take a window of 1 to 168 hours (default 24) and return one row per
(`placement_id`, `model_version`) seen on impression events in the window.

An impression SHALL be identified by the triple (`impression_id`, `placement_id`, `model_version`). The same
`impression_id` under a different placement or model version is a different impression, counted in its own row.

A click SHALL count only if its `impression_id` names an impression, with an impression event in the window at or
before the click, whose listing is the clicked listing. If the click carries a placement or a model version, the
impression's SHALL equal them. If several impressions qualify, the click SHALL be credited to the one with the latest
impression event (ties: placement, then model version, ascending), and to no other.

Each row has:
- `impressions`: distinct impressions (triples);
- `item_impressions`: impression events;
- `clicks`: counted clicks, as defined above;
- `add_to_carts`: add-to-cart events by the same `user_key` on a clicked listing, within
  `RECS_ATTRIBUTION_WINDOW_HOURS` after the click;
- `purchases`: paid order lines (order facts with status PAID) whose `buyer_id` equals the click's `user_key`, on a
  clicked listing, paid within `RECS_ATTRIBUTION_WINDOW_HOURS` after the click and not after the end of the report.
  A line is credited once, to the earliest attributable click. `purchase` tracking events SHALL NOT count, and order
  lines with no `buyer_id` SHALL NOT be attributed;
- `ctr`: clicks over item impressions;
- `conversion_rate`: purchases credited to mature clicks over mature clicks, 0 when there are no mature clicks. A click
  is mature when its attribution window had closed at the end of the report (click time plus
  `RECS_ATTRIBUTION_WINDOW_HOURS` is not after the end). Clicks whose window is still open are in `clicks` and their
  conversions are in `add_to_carts` and `purchases`, but not in `conversion_rate`.

Rows whose `model_version` is `serving-fallback` SHALL be reported like any other. The response SHALL also give, per
placement, the share of impressions served by the fallback. A window outside 1–168 hours SHALL fail with
`INVALID_ARGUMENT`. The RPC SHALL require the `admin` scope, and the gateway SHALL gate it at the edge.

#### Scenario: Impressions and clicks are counted per placement and model

- **WHEN** a visitor posts 2 impressions of a recommendation row (one `impressionId`, a placement P, model version M) and 1 click on one of its listings with the same `impressionId`
- **THEN** the admin's report for the last hour has a row for P and M with at least 1 impression, at least 2
  item impressions and at least 1 click

#### Scenario: A purchase after a recommended click is attributed

- **WHEN** a logged-in buyer clicks a recommended listing (impression id I, a placement P2, model version M2)
  and then pays an order for that listing
- **THEN** the report row for P2 and M2 counts that purchase

#### Scenario: A purchase without a recommended click is not attributed

- **WHEN** a logged-in buyer purchases a listing they never clicked from a recommendation row
- **THEN** no report row's purchase count includes that purchase

#### Scenario: A purchase beacon without an order is not counted

- **WHEN** a logged-in buyer clicks a recommended listing and posts a `purchase` tracking event for it, but pays no order
- **THEN** the report row for that placement and model counts the click and 0 purchases

#### Scenario: A click on a listing the impression did not show is not counted

- **WHEN** a visitor posts an impression of listing A and a click on listing B with the same `impressionId`
- **THEN** the report row for that placement and model counts the impression and 0 clicks

#### Scenario: A reused impression id keeps its placements and models apart

- **WHEN** a visitor posts impressions of one listing with the same `impressionId` under placement P4 with model
  M4 and under placement P5 with model M5, and one click on that listing carrying that `impressionId`, P4 and M4
- **THEN** the report has a row for P4 and M4 and a row for P5 and M5, each with at least 1 impression, and only the
  P4 and M4 row counts the click

#### Scenario: A conversion on a still-open attribution window is left out of the rate

- **WHEN** a logged-in buyer clicks a recommended listing and pays an order for it within the attribution window, and
  the report is read right away
- **THEN** the report row for that placement and model counts 1 click and 1 purchase, and its conversion rate is 0

#### Scenario: The fallback share is reported

- **WHEN** a visitor posts impressions for a placement P3 used by no other traffic, 1 with model version
  "serving-fallback" and 1 with a real model version
- **THEN** the report gives P3 a fallback share above 0 and below 1

#### Scenario: Only admins read recommendation performance

- **WHEN** an anonymous client and then a logged-in buyer call `GetRecommendationPerformance` through the gateway
- **THEN** the gateway answers HTTP 401 and then HTTP 403
