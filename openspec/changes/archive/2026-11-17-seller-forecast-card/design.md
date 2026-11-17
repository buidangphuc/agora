## Context

`page.tsx` already reads `getSellerFunnel` and `getRevenueBreakdown` with `Promise.allSettled` and hands nullable results to `AnalyticsView`. The forecast card follows the same pattern so a failing block never breaks the page. Server wrappers live in `src/lib/gateway/*.ts` and map bigint to number.

## Decisions

- Candidate listings: the top 5 `topSkus` by revenue of the selected range (U11). `getRevenueBreakdown` already accepts `from`/`to` in the proto, the wrapper currently does not pass them, so the page makes one extra range-scoped call used only to pick listings. The existing revenue/funnel calls and the client-side `daysInRange` filter are untouched, so no existing behaviour changes. Rows without a `listingId` are skipped (a SKU-only row cannot be forecast).
- State model, one per listing: `forecast` (call ok, `isColdStart=false`, 14 points), `cold` (call ok, `isColdStart=true`), `error` (call threw, including `Unavailable`). A response with `isColdStart=false` but zero points is treated as `error`, never as numbers; the card never fills gaps.
- Rendering rule: numbers appear only in the `forecast` state. The reorder point is shown rounded up to a whole unit because it is a count of units; the daily quantiles are shown with one decimal.
- Run date: `generatedAt` is formatted `dd/mm/yyyy` in UTC+7 next to the model version label as received (the wrapper does not rename or prettify `model_version`, so a baseline run is visibly a baseline).
- Provenance on beacons: the dispatcher forwards `impressionId` and `modelVersion` only from `items[]`, so for Magic Listing the beacon carries one item whose `itemId` is the listing id, or an empty string on the create form (no listing yet). Empty `request_id` or `model_version` from an older team-ai is omitted, never replaced by a placeholder.

## Risks

- Five sequential-looking reads could slow the page: they run in parallel and are bounded by the 30 ms server budget.
- `analytics-forecast-serving` not yet deployed: the card then shows the error state, not numbers, which is the intended safe failure.
