@analytics @seller @integration
Feature: GetDemandForecast serves the nightly job output from Redis
  AnalyticsQueryService.GetDemandForecast reads exactly one key `fc:v1:seller:<seller_id>:<listing_id>`
  in Redis DB 3 and returns its points, model_version and generated_at unchanged, truncated to the
  requested horizon. A missing key is a cold start with no numbers, a faulty payload is UNAVAILABLE.
  The steps seed and remove only the keys they create. (analytics-forecast-serving)

  Scenario: A forecast key is served as written
    Given a seller whose listing has a forecast key of 14 days with model_version "seasonal_naive_v1" generated at 2026-11-13T02:45:00Z
    When the seller calls GetDemandForecast for that listing
    Then the response has the 14 stored daily forecasts, model_version "seasonal_naive_v1", generated_at 2026-11-13T02:45:00Z and is_cold_start false

  Scenario: The horizon truncates and never pads
    Given a seller whose listing has a forecast key of 14 days with model_version "seasonal_naive_v1" generated at 2026-11-13T02:45:00Z
    When the seller calls GetDemandForecast for that listing with horizon_days 7
    Then the response has the first 7 stored daily forecasts
    When the seller calls GetDemandForecast for that listing with horizon_days 40
    Then the response has the 14 stored daily forecasts

  Scenario: No key means cold start without points
    Given a seller with a paid order line for a listing that has no forecast key
    When the seller calls GetDemandForecast for that listing
    Then the response is a cold start with no daily forecasts, reorder point 0, safety stock 0 and no generated_at

  Scenario Outline: A malformed payload returns UNAVAILABLE
    Given a seller whose listing key holds <payload>
    When the seller calls GetDemandForecast for that listing
    Then the call fails with code "unavailable" and carries no forecast numbers

    Examples:
      | payload                      |
      | invalid JSON                 |
      | a point with p10 above p50   |

  Scenario: Another seller's forecast is denied
    Given a seller whose listing has a forecast key of 14 days with model_version "seasonal_naive_v1" generated at 2026-11-13T02:45:00Z
    And another seller exists
    When the other seller calls GetDemandForecast for the first seller's listing
    Then the call fails with code "permission_denied" and carries no forecast numbers
    When an anonymous caller calls GetDemandForecast for the first seller's listing
    Then the call fails with code "unauthenticated" and carries no forecast numbers
    When an admin calls GetDemandForecast for the first seller's listing
    Then the response has the 14 stored daily forecasts

  Scenario: A read stays within the latency budget
    Given a seller whose listing has a forecast key of 14 days with model_version "seasonal_naive_v1" generated at 2026-11-13T02:45:00Z
    When the seller calls GetDemandForecast for that listing 200 times in sequence
    Then the 95th percentile call latency is at most 30 ms
