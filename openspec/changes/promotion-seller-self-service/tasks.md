## 1. Code track (team-promotion, then team-gateway)

- [ ] 1.1 Vendor the `seller-centre-contract` proto into `team-promotion/proto` and `team-gateway`, regenerate; confirm the four RPCs and `FlashSaleCampaign.seller_id` exist (blocked until that slice merges)
- [ ] 1.2 Migration `0009_campaign_seller.{up,down}.sql` and `seller_id` on `store.FlashSaleCampaign` and `campaignSelectColumns`; create stores the listing owner (scenarios: A new campaign records the listing owner, Migration keeps old rows valid)
- [ ] 1.3 `ListMyCampaigns` in the repository, `flashsale.Service` and `FlashSaleHandler` (scenarios: Seller sees only their own campaigns with status, Status filter narrows the list, Paging boundary at 21 campaigns, Caller without listing.write is denied, Anonymous caller is rejected)
- [ ] 1.4 `CancelIfScheduledOwned`, `ErrNotOwner` mapping and `CancelMyCampaign` handler, no audit (scenarios: Seller cancels their SCHEDULED campaign, Cancelling an ACTIVE campaign is refused, Cancelling another seller's campaign is denied like an unknown id, Cancelling twice is idempotent, Campaign starting during the cancel is not cancelled, Seller cancellation writes no audit event)
- [ ] 1.5 `ListMyShopVouchers` and `DeactivateMyShopVoucher` in `voucher.Service` and `VoucherHandler` (scenarios: Seller sees own vouchers of every state, Empty list for a seller with no vouchers, Buyer without listing.write is denied, Page size above the maximum is capped, Seller deactivates their voucher, Deactivating twice keeps the first timestamp, Deactivating another seller's voucher is denied like an unknown id, A platform voucher cannot be deactivated by a seller, Seller deactivation writes no audit event)
- [ ] 1.6 `backfill-campaign-sellers` subcommand (scenarios: Backfill fills old campaigns once, Deleted listing is left unassigned, Team-domain down stops the backfill)
- [ ] 1.7 team-gateway forwarders for the four RPCs; `policy.go` unchanged (scenarios: Seller reaches the seller RPCs but not the admin ones, Promotion down surfaces as unavailable)
- [ ] 1.8 Update the `team-promotion/README.md` authorization table; gates through `gate_digest.py` with `GO_SDK_REQUIRE_DOCKER=1`; `openspec validate promotion-seller-self-service --strict`

## 2. e2e track (platform-e2e)

- [ ] 2.1 `features/promo/promotion_seller_self_service.feature` and steps for every platform-e2e scenario above, seeding campaigns and vouchers through gateway API (mock data, no fake services); includes the audit-stopped scenarios (Seller cancellation does not depend on audit, Seller deactivation does not depend on audit)
- [ ] 2.2 Regression: existing admin cancel and deactivate scenarios (Admin cancel keeps its admin rules, Admin deactivate keeps its admin rules) still pass unchanged
- [ ] 2.3 Backfill e2e-level check on a seeded stack: run the subcommand twice (scenario: Backfill fills old campaigns once)
