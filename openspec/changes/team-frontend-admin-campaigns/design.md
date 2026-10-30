## Context

`FlashSaleService` and `SubscriptionService` are forwarded by `team-gateway` (`internal/edge/promotion.go`, `callWrite` for `CreateCampaign`, `callRead` for the rest). `team-promotion` enforces: `CreateCampaign` needs a principal with `admin` or `listing.write`; an admin create skips the listing ownership check and runs inside `auditx.Gate.Do` (event `flash_sale.admin_create`), which returns `UNAVAILABLE` "audit trail unavailable" and does not insert when the audit write fails. `ListActiveCampaigns`, `GetActiveFlashSale` and `ListPlans` need no scope. The gateway replaces the message of an `UNAVAILABLE` error, so the app cannot tell audit-down from promotion-down.

## Decisions

- One wrapper module `src/lib/gateway/campaigns.ts` following `users.ts`: per-request clients from `makeClients(getToken())`, `int64` and `Timestamp` mapped to numbers and ISO strings, results keyed by code (`unauthenticated`, `forbidden`, `invalid`, `not_found`, `timeout`, `unavailable`).
- Tests of the wrapper run the generated clients over `createRouterTransport` (an in-memory Connect router), so request encoding, `bigint` fields and error codes go through the real protobuf and Connect code paths.
- Two-step confirm: the form submit only validates and opens a review dialog with a summary; "Xác nhận tạo" sends the Server Action. The success view renders the campaign from the response (id, stock sold), not the typed input.
- The Server Action re-parses the raw draft (strings) with the same `parseCampaignDraft`, so the server never trusts client validation. Times are entered as `datetime-local` in Vietnam time (UTC+7), as in the users screen.
- `UNAVAILABLE` keeps the dialog open with "nothing was created, try later"; a timeout closes it and asks the operator to check the active list first, because `CreateCampaign` is not idempotent (G10).
- Plans live on their own route `/admin/plans` with their own nav entry; campaigns reuse the existing hidden `/admin/campaigns` entry, now shown. The voucher entry is left as it was.

## Risks

- Gaps doc IDs G9-G11 and the shared nav/test lines may collide with the parallel voucher change (E1c); resolve by renumbering on merge.
- Not yet run against the real promotion, audit and gateway services; covered by fake-transport tests and a browser check against an unreachable gateway.
