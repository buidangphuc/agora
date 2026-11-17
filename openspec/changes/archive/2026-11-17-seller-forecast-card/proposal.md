## Why

`/seller/analytics` shows funnel and revenue only. The `GetDemandForecast` RPC has no caller, and `analytics.demand-forecast` in team-analytics FEATURES.yaml already claims entry route `/seller/analytics`. Sellers cannot plan restocks from data. Once `analytics-forecast-serving` serves honest numbers (no key means a cold start with no points, a store error means `UNAVAILABLE`), this slice shows them as a "Dự báo nhu cầu 14 ngày" card and never invents a number itself. The same slice makes the seller Magic Listing beacons carry the `request_id` and `model_version` that team-ai now returns, so the AI-surface report can group seller suggestions by model.

initiative: docs-ai-data

## What Changes

All code is in `team-frontend-seller`; no proto change (the contract slice `docs-ai-data-contract` already added `generated_at`, `request_id`, `model_version`).

- `src/lib/gateway/analytics.ts`: new server-only `getDemandForecast(sellerId, listingId, opts)` over `AnalyticsQueryService.GetDemandForecast` with `horizonDays=14`, mapped to a plain view type (`ViewDemandForecast`: daily p10/p50/p90, `suggestedReorderPoint`, `safetyStock`, `modelVersion`, `generatedAt` as ISO string or null, `isColdStart`). A `ConnectError` with code `Unavailable` (or any thrown error) is surfaced to the caller, never turned into zeros. `getRevenueBreakdown` gains an optional `{ from, to }` so the card can pick the top listings of the selected range; the existing page call keeps its current arguments.
- `src/app/seller/analytics/page.tsx`: after the revenue read, takes the top 5 listings by revenue of the selected range (decision U11), reads their forecasts in parallel with `Promise.allSettled`, and passes per-listing results to a new `ForecastCard`. A forecast failure never fails the page; the funnel and revenue blocks keep their own Alerts.
- `src/features/seller/ForecastCard.tsx` (new, server-renderable): title "Dự báo nhu cầu 14 ngày"; one row per listing with three states. Forecast: a 14-row table of date, p10, p50, p90, the suggested reorder point ("Điểm đặt hàng lại: N đơn vị", rounded up), the model version and the run date. Cold start: "Chưa đủ dữ liệu để dự báo" and no numbers. Error: "Dự báo tạm thời không khả dụng" and no numbers. When the seller has no top listings the card shows an `Empty` "Chưa có đơn hàng để dự báo".
- `src/features/listing/MagicListingAssist.tsx`, `src/features/listing/actions.ts`, `src/lib/gateway/ai.ts`, `src/lib/analytics/ai.ts`: `ViewMagicListing` gains `requestId` and `modelVersion`; `trackAiInteraction` gains an optional provenance argument that puts `impressionId = requestId` and `modelVersion` on the beacon item (the only place the dispatcher forwards them); `properties` stay exactly `surface` and `action`.
- `platform-e2e`: new `features/seller/seller_forecast_card.feature` and step definitions; seed a seller with at least 14 days of paid order lines plus a forecast run, and one seller with orders but no forecast key.
- Owner `FEATURES.yaml` entries are added by the e2e-author, not here.

## Capabilities

### New Capabilities
- `seller-demand-forecast-card`: the forecast card on `/seller/analytics` with its three honest states.
- `seller-magic-listing-provenance`: Magic Listing `ai_interaction` beacons carry the AI `request_id` and `model_version`.

### Modified Capabilities
none (`seller.analytics-range` behaviour, URL range tabs and Alerts stay as they are)

## Impact

- team-frontend-seller only, plus platform-e2e. Depends on `analytics-forecast-serving` (real `fc:v1` backed responses and `generated_at`) and `ai-copilot-grounding` (Magic Listing `request_id` and `model_version`). Uses `AnalyticsQueryService.GetDemandForecast` and `AIService.MagicListing` unchanged.
- No new dependency, port, env var or route. The gateway already routes `GetDemandForecast` for a seller scope; if it does not, that is a gap to raise (see tasks 1.2).

## Non-goals

Per-listing funnel table and the funnel range fix (separate backlog item). Photo-aware Magic Listing, `image_url` use and attributes (initiative decision U1). Chart rendering of the forecast (a table is enough). Any forecast computed or smoothed in the frontend. Reorder-point math (stays in team-analytics `internal/rpc/handler.go`). Chat copilot UI in the seller app (none exists).

## Non-functional requirements

- Latency: the card adds at most 5 parallel `GetDemandForecast` calls (each p95 at most 30 ms server side), so the page TTFB grows by at most one such round trip plus gateway overhead; the sum must not push `/seller/analytics` p95 server render over 1.5 s on the local stack.
- Availability: any forecast error is confined to its row; the page returns 200 with the funnel and revenue blocks intact.
- Limits: at most 5 forecast reads per page render; `horizon_days` fixed at 14.
- Accessibility: the table has a caption and the three states are text, not colour only.
- Language: Vietnamese copy; dates `dd/mm/yyyy` in UTC+7.

## Data classification

| field | personal data | purpose | consent basis | retention |
|---|---|---|---|---|
| Forecast p10/p50/p90, reorder point, safety stock, model version, run date | no: per-listing aggregate of the seller's own sales | restock planning for that seller | contract (seller operating their own shop); no analytics consent involved | not stored by the frontend; rendered per request, no client cache |
| `seller_id` in the request | pseudonymous id | scope the read to the caller | contract | not logged by this repo (principal id is read server-side, never from the URL) |
| Magic Listing beacon `impressionId` (= `request_id`), `modelVersion` | no: random UUID and a model label | tie a seller AI interaction to the response | as other first-party events (decision U6, DPO sign-off is a human item) | as the `analytics.events` pipeline |
| Beacon `properties` | no: exactly `surface`, `action`; never listing text | AI-surface report | as above | as above |

## Threat model

- Assets: a seller's sales forecast (commercially sensitive); the integrity of the shown numbers (a made-up forecast drives real restock spending).
- Abuse cases: a seller reading another seller's forecast by tampering with a query string or form field; a store failure rendered as plausible numbers; a stale or cold-start response rendered as if it were a forecast; AI free text leaking into beacons.
- Authorisation rule: `GetDemandForecast` is called server-side with the `seller_id` of the session principal and the listing ids taken from that seller's own revenue breakdown; no request parameter names a seller or a listing for the card. The gateway and team-analytics enforce seller scope. The card page requires the existing `listing.write` scope guard.
- Honesty rule: numbers render only when `isColdStart` is false and the call succeeded; every other outcome renders text only.
