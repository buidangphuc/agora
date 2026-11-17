## Why

A seller can create a flash-sale campaign and a shop voucher but cannot list or stop them: `ListCampaigns`,
`CancelCampaign` and `DeactivateVoucher` are admin-only at the edge (`team-gateway/internal/edge/policy.go:42-46`)
and `flash_sale_campaigns` has no seller column (`0001_promotion.up.sql:42-53`). The seller marketing hub
(`seller-marketing-hub`) and the first-steps card (`seller-getting-started`) need seller-scoped reads and stops.
Opening the admin RPCs would drop their admin edge policy and audit-first rule (AGENTS.md section 3.6), so this
slice adds four seller RPCs next to them.

initiative: seller-centre

## What changes

- team-promotion implements four RPCs declared by `seller-centre-contract`: `FlashSaleService.ListMyCampaigns`,
  `FlashSaleService.CancelMyCampaign`, `VoucherService.ListMyShopVouchers`, `VoucherService.DeactivateMyShopVoucher`.
  The seller always comes from the principal, never from a request field; each needs scope `listing.write`.
- Migration `0009`: `flash_sale_campaigns.seller_id TEXT NOT NULL DEFAULT ''` plus index
  `(seller_id, starts_at DESC)`. Every `CreateCampaign` stores the listing owner returned by team-domain
  `ListingService.GetListing` (the lookup the seller path already makes) and `FlashSaleCampaign.seller_id` is
  returned server-owned.
- New subcommand `team-promotion backfill-campaign-sellers`: one-shot, idempotent, fills rows whose `seller_id` is
  `''` through `ListingService.GetListing`.
- A seller may cancel only a SCHEDULED campaign; the state check and the write are one locked transaction.
- team-gateway forwards the four RPCs (`VoucherForwarder`, `FlashSaleForwarder`). `adminProcedures` in `policy.go`
  is not touched: the new procedures are not in it, the service enforces `listing.write` and ownership.
- Seller calls are not admin mutations and write no audit event; the admin RPCs and their audit stay as they are.
- e2e: API-level scenarios through the gateway with real team-promotion and team-domain.

Out of scope: the seller-app UI (`seller-marketing-hub`), removing the create form from the buyer `/vouchers`
page (`seller-marketing-hub`), any voucher or flash-sale pricing rule, any admin-app change.

## Capabilities

### New Capabilities
- `promotion-seller-self-service`: seller-scoped listing and stopping of own shop vouchers and flash-sale campaigns.

### Modified Capabilities
none (admin capabilities `flash-sale-admin`, `promotion-admin-vouchers` are unchanged).

## Impact

- team-promotion: `internal/rpc/flashsale.go`, `internal/rpc/voucher.go`, `internal/flashsale/`, `internal/voucher/`,
  `internal/store/` (new migration `0009_campaign_seller.{up,down}.sql`, campaign repository), `internal/app/execute.go`,
  `proto/` vendored copy, `README.md` authorization table, `FEATURES.yaml`.
- team-gateway: `internal/edge/promotion.go` (four forwarder methods; hot file shared with `order-bundle-checkout`),
  vendored proto.
- platform-e2e: feature files and steps for the scenarios below.
- Depends on `seller-centre-contract` being merged in `platform-core/packages/proto` first.

## Non-functional requirements

- Latency: `ListMyCampaigns`, `ListMyShopVouchers` p95 <= 150 ms locally at 2,000 campaigns / 2,000 vouchers per
  seller (index `(seller_id, starts_at DESC)`; vouchers already filter `seller_id`). Page size default 20, max 200
  (existing campaign/voucher paging). `CancelMyCampaign`, `DeactivateMyShopVoucher` p95 <= 100 ms.
- Availability: list reads need only promotion_db; team-domain is needed only on create and in the backfill. No
  `*_ENABLED` flag and no in-memory fallback: a down promotion_db fails the call with UNAVAILABLE.
- Limits: backfill processes rows in batches of 100 and is safe to re-run or interrupt.
- Migration is additive and online (nullable-free default `''`); down migration drops the index and column.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| `flash_sale_campaigns.seller_id` | pseudonymous account id, personal data | list a seller's own campaigns | contract (seller operating the shop) | with the campaign row, as today |
| `vouchers.seller_id` | same, existing | list a seller's own vouchers | contract | unchanged |
| voucher code, quota, used | business data, not personal | shown to the owning seller | contract | unchanged |

No new personal data class is introduced. Seller and buyer ids are never logged raw (sloglint forbidden-keys);
backfill logs counts only, never ids.

## Threat model

- Assets: a seller's campaigns and vouchers (price manipulation, availability of promotions), the admin RPC surface.
- Abuse cases: (1) seller B cancels or deactivates seller A's item by guessing an id; (2) seller probes which ids
  exist; (3) a buyer or anonymous caller reaches the seller RPCs; (4) a seller cancels a live (ACTIVE) flash sale to
  disrupt buyers mid-sale or to dodge a price commitment; (5) a seller passes another `seller_id` in the request;
  (6) a seller escalates to admin RPCs through these.
- Authz rule: principal required, scope `listing.write` required (else PERMISSION_DENIED; anonymous UNAUTHENTICATED);
  the owner is the principal id; a foreign, unknown, empty-seller or platform-scope id answers PERMISSION_DENIED with
  one message (no existence oracle); the request has no seller field; admin edge policy unchanged; gateway adds no
  scope bypass.
