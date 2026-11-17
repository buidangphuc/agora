# seller-demand-forecast-card Specification

## Purpose
Defines the 14-day demand forecast card on `/seller/analytics`, which shows served forecast numbers, cold-start and unavailable states, and never invents a number.

## Requirements

### Requirement: Forecast card shows real forecasts
The `/seller/analytics` page SHALL render a card titled "Dự báo nhu cầu 14 ngày" for the top 5 listings by revenue in the selected range, and for each listing whose forecast call succeeded with `is_cold_start` false SHALL show the 14 daily p10, p50 and p90 values, the suggested reorder point, the model version and the run date, all taken from `GetDemandForecast` with `horizon_days=14`.

#### Scenario: Seller with history sees the forecast
- **WHEN** a logged-in seller with at least 14 days of paid order lines on listing L and a forecast run after them opens `/seller/analytics`
- **THEN** the card lists L with 14 rows of date, p10, p50 and p90, a line "Điểm đặt hàng lại: N đơn vị" equal to the RPC `suggested_reorder_point` rounded up, the RPC `model_version` string and the `generated_at` date as `dd/mm/yyyy`
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature` `Seller with history sees the forecast`

#### Scenario: Values are rendered exactly as served
- **WHEN** the gateway returns a forecast whose p10, p50 and p90 are 1.0, 2.5 and 4.0 for a date
- **THEN** that date row shows 1.0, 2.5 and 4.0 and no value is recomputed, smoothed or clamped by the frontend
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ForecastCard.test.tsx` "renders served quantiles unchanged"

#### Scenario: Card covers only the top five listings of the range
- **WHEN** the seller has paid revenue on 7 listings in the selected range
- **THEN** the card shows exactly 5 listing sections, ordered by revenue descending, and `getDemandForecast` is called exactly 5 times
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "forecasts only the top five listings of the range"

#### Scenario: Range tab selects the candidate listings
- **WHEN** the seller selects the "30 ngày" range tab
- **THEN** the URL contains `range=30d` and the revenue breakdown used to pick listings is requested with a `from` 30 days before `to`
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "range tab scopes the listing pick"

#### Scenario: Forecast request is fixed at fourteen days
- **WHEN** `getDemandForecast` is called by the page
- **THEN** the RPC request has `horizon_days=14` and the `seller_id` of the session principal
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/analytics.test.ts` "requests a 14 day horizon for the principal"

#### Scenario: Gateway rejects the forecast call without a token
- **WHEN** `getDemandForecast` is called with a session token that the gateway rejects with `Unauthenticated`
- **THEN** the wrapper rejects with that error and the card shows "Dự báo tạm thời không khả dụng" without numbers
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/analytics.test.ts` "rejects on Unauthenticated"

### Requirement: Cold start shows no numbers
The card SHALL show the text "Chưa đủ dữ liệu để dự báo" and no numeric forecast, reorder point or run date for a listing whose response has `is_cold_start` true.

#### Scenario: Listing without a forecast key
- **WHEN** a seller has paid orders on listing L but the forecast job wrote no key for L, so the RPC returns `is_cold_start=true` with no points
- **THEN** L's section reads "Chưa đủ dữ liệu để dự báo" and contains no digit-bearing forecast cell, no reorder point and no model version
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature` `Listing without a forecast key`

#### Scenario: Cold start with stray points is not rendered
- **WHEN** a response has `is_cold_start=true` and a non-empty `daily_forecasts` list
- **THEN** the section shows only "Chưa đủ dữ liệu để dự báo" and none of the points
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ForecastCard.test.tsx` "cold start never renders points"

#### Scenario: Warm response with no points is an error state
- **WHEN** a response has `is_cold_start=false` and zero `daily_forecasts`
- **THEN** the section reads "Dự báo tạm thời không khả dụng" and shows no numbers
- **VERIFIED BY**: `team-frontend-seller/src/features/seller/ForecastCard.test.tsx` "empty warm response is treated as an error"

### Requirement: Forecast errors show no numbers and stay local
The card SHALL show the text "Dự báo tạm thời không khả dụng" and no numbers for a listing whose `GetDemandForecast` call failed, including code `Unavailable`, and a forecast failure SHALL NOT change the status or content of the funnel and revenue blocks.

#### Scenario: Store unavailable
- **WHEN** team-analytics cannot read the forecast store and `GetDemandForecast` returns `Unavailable` for every listing
- **THEN** every listing section reads "Dự báo tạm thời không khả dụng", no forecast number is shown, and the page responds 200
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature` `Store unavailable`

#### Scenario: One listing fails, others render
- **WHEN** the call for the second of three listings rejects and the other two succeed
- **THEN** the second section shows "Dự báo tạm thời không khả dụng" and the other two show their forecasts
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "one failed forecast does not hide the others"

#### Scenario: Forecast failure leaves funnel and revenue intact
- **WHEN** every forecast call rejects while the funnel and revenue reads succeed
- **THEN** the KPI row, the funnel block and the revenue table render with their data and no funnel or revenue Alert appears
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "forecast failure does not touch funnel and revenue"

#### Scenario: Wrapper surfaces failure instead of zeros
- **WHEN** the gateway call in `getDemandForecast` throws a `ConnectError` with code `Unavailable`
- **THEN** the wrapper rejects with that error and returns no view object
- **VERIFIED BY**: `team-frontend-seller/src/lib/gateway/analytics.test.ts` "rejects on Unavailable"

#### Scenario: Revenue read fails
- **WHEN** the revenue breakdown used to pick listings fails
- **THEN** the card shows an `Alert` "Dự báo tạm thời không khả dụng" with a "Thử lại" link to the same URL and no forecast call is made
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "revenue failure shows the forecast alert"

### Requirement: Empty seller sees an empty state
The card SHALL show an `Empty` state "Chưa có đơn hàng để dự báo" and make no `GetDemandForecast` call when the selected range has no listing with revenue.

#### Scenario: New seller without orders
- **WHEN** a seller with no paid orders in the range opens `/seller/analytics`
- **THEN** the card shows "Chưa có đơn hàng để dự báo" and zero forecast calls are made
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "no revenue means an empty forecast card"

#### Scenario: Top SKU without a listing id is skipped
- **WHEN** a top SKU row has an empty `listing_id`
- **THEN** no forecast call is made for it and no section is shown for it
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "skips SKU rows without a listing id"

### Requirement: Forecast is scoped to the logged-in seller
The page SHALL read forecasts only for the session principal's own id and for listing ids that came from that seller's own revenue breakdown, and SHALL NOT accept a seller or listing id from the URL; a visitor without a session or without scope `listing.write` SHALL NOT trigger any forecast call.

#### Scenario: Query string cannot select another seller
- **WHEN** a seller opens `/seller/analytics?range=30d&sellerId=other&listingId=other-1`
- **THEN** every `GetDemandForecast` request carries the principal's own `seller_id` and none carries `other` or `other-1`
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "ignores seller and listing ids from the URL"

#### Scenario: Guest is redirected
- **WHEN** a request without a session opens `/seller/analytics`
- **THEN** the response redirects to login and no `GetDemandForecast` call is made
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "guest triggers no forecast call"

#### Scenario: Buyer without seller scope is denied
- **WHEN** a logged-in principal without scope `listing.write` opens `/seller/analytics`
- **THEN** the page renders nothing and no `GetDemandForecast` call is made
- **VERIFIED BY**: `team-frontend-seller/src/app/seller/analytics/analytics.test.tsx` "no listing.write scope triggers no forecast call"

#### Scenario: Gateway rejects a foreign seller id
- **WHEN** a seller session calls `GetDemandForecast` through the gateway with another seller's id
- **THEN** the call is denied with `PermissionDenied` and no forecast is returned
- **VERIFIED BY**: `platform-e2e/tests/e2e/features/seller/seller_forecast_card.feature` `Gateway rejects a foreign seller id`
