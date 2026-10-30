## Why

The admin app (ADR-0006) holds the operator actions that touch the whole marketplace. Plan step E1d adds flash-sale campaigns and the seller plan catalogue: an operator can start a flash sale for any listing, see which sales are running and read the plans sellers can buy, all on the admin host.

## What Changes

- `team-frontend-admin/src/lib/gateway/campaigns.ts`: server-only wrappers over `FlashSaleService.CreateCampaign`, `ListActiveCampaigns`, `GetActiveFlashSale` and `SubscriptionService.ListPlans`, returning typed results by Connect code and never the upstream message. `makeClients` gains `flashSale` and `subscription`.
- `/admin/campaigns`: active campaigns (page size 20, `?cursor=`), a lookup of one listing's active sale (`?listing=`), a link to create, and a note that scheduled or ended campaigns, cancel and edit are not available.
- `/admin/campaigns/new`: form validated on the client and again in the Server Action, a review dialog with a summary (two-step confirm), then the campaign as returned by the server. Errors map by code; `UNAVAILABLE` (audit trail or promotion down, both fail-closed) says nothing was created; a timeout says the outcome is unknown and points to the active list.
- `/admin/plans`: read-only plan catalogue.
- Admin nav shows "Chiến dịch" and a new "Gói đăng ký" entry.
- Gaps G9-G11 in `fe-platform/docs/gaps.md` and the admin README "Known gaps".

## Capabilities

### New Capabilities
none

### Modified Capabilities
none

`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `team-frontend-admin/src` (gateway wrapper, feature components, three routes, nav), `team-frontend-admin/README.md`, `fe-platform/docs/gaps.md`.
- No proto, gateway or service change; RPCs and scopes confirmed in `platform-core/packages/proto/platform/promotion/v1/promotion.proto`, `team-gateway/internal/edge/promotion.go` and `team-promotion/internal/rpc/{flashsale,subscription,common}.go`.

## Non-goals

Platform vouchers (E1c), cancelling or editing a campaign, listing scheduled or ended campaigns, subscribing a seller, backend changes, layout pass (plan F).

## Non-functional requirements

`npm run check` and `npm run build` green in `team-frontend-admin`; every new page declares `referrer: no-referrer`; no backend message reaches the UI or a URL; controls at least 44 px; no horizontal scroll at 375 px.
