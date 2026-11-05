# Proposal: promotion-admin-campaigns

initiative: p10-backend-gaps

## Why
Closes gaps G12, G13, G14 in `fe-platform/docs/gaps.md`. Admin `/admin/campaigns` can only see active campaigns and cannot cancel or edit one. A retried `CreateCampaign` after a timeout can create two campaigns. The server validates only `listing_id`, the admin path skips the listing lookup, and the kill-switch silently empties reads.

## What changes
- team-promotion `internal/flashsale` + `internal/rpc`: new `FlashSaleService.ListCampaigns`, `CancelCampaign`, `UpdateCampaign` (admin scope only); `CreateCampaign` gains required `idempotency_key` (admin path), server validation on seller and admin paths via `ListingService.GetListing`; `ListActiveCampaigns` and `ListCampaigns` return `flash_sale_disabled`; `ReserveFlashSaleStock` refuses a cancelled campaign with the existing reason `not_active`.
- Campaign status derives from `cancelled_at` and the window (cancelled > scheduled/active/ended). New columns: `flash_sale_campaigns.cancelled_at`; new table for idempotency records `(actor_id, idempotency_key)` unique with payload hash and `created_at`, purged after 24 h.
- Every admin write (cancel, update, create) writes its team-audit entry first (`campaign.cancel`, `campaign.update`, existing `flash_sale.admin_create`) and fails closed `UNAVAILABLE`.
- team-gateway `internal/edge/promotion.go` and scope table: expose the three RPCs, admin scope only (hot file shared with promotion-admin-vouchers).
- team-frontend-admin: `/admin/campaigns` lists all campaigns with status filter, paging, cancel and edit (scheduled only); `/admin/campaigns/new` sends a stable idempotency key per form session and retries safely; kill-switch banner.
- team-frontend is not touched: the buyer flash-sale empty state is dropped (`FlashSaleBlock` is a stub that calls no campaign list). The user confirmed the drop on 2026-11-02.
- `fe-platform/docs/gaps.md`: rows G12, G13, G14 removed.

Depends on `p10-gaps-contract` (proto: `CampaignStatus`, three RPCs, `idempotency_key = 7`, `flash_sale_disabled = 3`, `status = 9`, `cancelled_at = 10`).

## Non-goals
Hard delete of campaigns; editing `listing_id` or `variant_id`; changing stock already sold; buyer-side campaign list.

## NFR
- ListCampaigns p95 < 200 ms for a page of 50 locally; page size default 20, max 200.
- Idempotency lookup runs before validation and the listing lookup; a hit returns the campaign's current stored state.
- Perf: p95 figure is informational for local runs; checked by team-promotion `internal/flashsale` `BenchmarkListCampaignsPage50`, not a gate.
- Idempotency record kept 24 h; replay returns the stored campaign without a second audit entry or outbox event.
- Cancel takes effect for reserve within the same transaction: no hold is granted after the cancel call returns.
- Listing lookup and audit use the existing `UPSTREAM_CALL_TIMEOUT_SECONDS` (2 s); outage returns within 2.5 s.

## Data classification
- Admin actor id on idempotency record and audit entries: purpose retry safety and accountability; basis legitimate interest (internal operations) - HUMAN ITEM: DPO must confirm this basis; retention idempotency row 24 h, audit rows per team-audit policy.
- Payload hash (SHA-256 of listing, variant, price, cap, window): not personal; same 24 h.
- Campaign fields (listing id, price, stock, window): business data, no personal data.

## Threat model
- Assets: campaign integrity (price, stock cap), audit trail, kill-switch state.
- Abuse cases: non-admin or seller cancels/edits/lists all campaigns; replay with a changed payload under an old key to overwrite; price at or above listing price to mislead buyers; editing a live campaign to change price under holders; idempotency key collision across admins; skipping audit via audit outage.
- Authz rule: ListCampaigns, CancelCampaign, UpdateCampaign require scope `admin` (seller and anonymous get PERMISSION_DENIED / UNAUTHENTICATED). Seller `CreateCampaign` stays ownership-checked through `listing.Owner`. Idempotency keys are scoped per admin principal. Audit written before the write; audit down means nothing changes.

## Error identification
Audit outage on create, cancel and update returns UNAVAILABLE with `google.rpc.ErrorInfo{domain:"promotion.platform", reason:"AUDIT_UNAVAILABLE"}`. Listing outage returns UNAVAILABLE with `ErrorInfo{domain:"promotion.platform", reason:"LISTING_UNAVAILABLE"}`.
