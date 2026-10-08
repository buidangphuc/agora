## Purpose

Defines the online success signal of recommendations: what analytics computes from exposure logs
(impressions and clicks carrying `placement_id`, `request_id`, `model_version`, `position`), how it is read,
and how a challenger model is compared with the current one on real traffic.

## ADDED Requirements

### Requirement: Analytics computes online metrics per placement and model version

`team-analytics` SHALL compute, from impression and click events carrying attribution fields, for a time
window and for each `placement_id` and `model_version` (and each model family, the version without its
timestamp suffix): impressions (distinct `request_id` + listing pairs), attributed clicks (clicks whose
`request_id` + listing pair had an impression), CTR (attributed clicks / impressions), coverage (distinct
impressed listings / distinct published listings known to analytics in the window) and novelty (mean
self-information of impressed listings by their share of views in the window). A click without a matching
impression SHALL NOT count toward CTR. Metrics SHALL be aggregates only and SHALL carry no user identifier.

#### Scenario: CTR counts only attributed clicks

- **WHEN** a placement served one request with five impressions, one of its cards is clicked twice, and a
  click arrives with an unknown `request_id`
- **THEN** that placement and model version report 5 impressions, 1 attributed click and CTR 0.2

#### Scenario: Two models on one placement are reported separately

- **WHEN** impressions and clicks on `home.for_you` carry two different model versions
- **THEN** the metrics hold one row per model version with its own impressions, clicks, CTR, coverage and
  novelty

### Requirement: Online metrics are readable by administrators

`AnalyticsQueryService.GetPlacementMetrics` SHALL return the online metrics for a window, optionally
filtered by `placement_id`, `model_version` or `request_id`; with a `request_id` it SHALL return that
request's impression and attributed-click counts. The RPC SHALL require the `admin` scope, enforced by
team-analytics and by the gateway's admin-only procedure set; any other caller SHALL be refused.

#### Scenario: An impression and a click land in analytics joined by request id

- **WHEN** a buyer sees the "Gợi ý cho bạn" row and opens one of its cards, and analytics has ingested the
  events
- **THEN** `GetPlacementMetrics` filtered by that response's `request_id` reports at least one impression
  and exactly one attributed click for `home.for_you` and the served `model_version`

#### Scenario: A buyer cannot read online metrics

- **WHEN** a buyer calls `GetPlacementMetrics` through the gateway
- **THEN** the call fails with `PERMISSION_DENIED`

### Requirement: A challenger is compared online before it replaces the current model

A new model family SHALL reach all users only through an online comparison: it is published as a challenger
generation that has passed the offline gate, served to a hash-assigned share of users, and promoted to
current (or stopped) by an explicit operator action after reading `GetPlacementMetrics` by model family.
Nightly retraining of the current family SHALL keep promoting through the offline gate alone.

#### Scenario: Promoting the challenger makes it current for everyone

- **WHEN** an operator promotes a running challenger
- **THEN** the challenger becomes current, the former current becomes the rollback target, the challenger
  pointer is cleared, and every `Recommend` response carries the promoted `model_version`
