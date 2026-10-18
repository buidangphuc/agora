## Decisions

1. **Where the audit call lives.** In the service layer (`VoucherService`, `FlashSaleService`, `SponsoredService`) behind an `AuditWriter` interface, set with `WithAudit(w)`; `cmd/server/main.go` calls it only when `NewAuditClient` returned non-nil. A nil writer on an admin path returns `Unavailable` (fail closed). The handlers keep authn/authz and pass the verified actor id in the params (`AuditActorID`/`CallerID`).
2. **Order of operations.** authn -> scope/role check -> input validation -> audit write -> insert -> event emission. A refused or invalid call never reaches the audit write.
3. **Voucher target = code.** `CreateVoucherRequest.code` is required, trimmed-non-empty and exists before creation. Metadata: `discount_type` = the `DiscountType` enum name (for example `DISCOUNT_TYPE_PERCENT`), `value` = `discount_value` in decimal. An UNSPECIFIED scope is audited like PLATFORM because the existing handler already gates it as platform.
4. **Campaign target = generated id.** Checked in code: `PostgresFlashSaleRepository.Create`, `PostgresAdCampaignRepository.Create` and their in-memory twins do `if c.ID == "" { c.ID = newID() }`, so a preset id is kept. The service therefore generates a 32-char hex id (crypto/rand, same scheme as `repository.newID`, which is unexported) before the audit write and sets it on the entity. The event target is the same id the response returns, so "who touched this campaign" is answerable. No name is used (campaigns have none).
5. **Uniform admin rule for campaigns (human answer, Round 1).** Neither campaign request carries a seller id, so "for another seller" cannot be decided without a team-domain lookup. Every `CreateCampaign` / `CreateAdCampaign` call by a caller holding `admin` is audited; there is no owner lookup and no new upstream call. Metadata: flash sale has no seller field and none is resolved, so it records `listing_id` instead of `seller_id`; ad campaign records `seller_id` (the value the service stores, the calling principal's id) plus `listing_id` and `budget`. Non-admin sellers keep `requireListingOwner` and are not audited.
6. **Behaviour unchanged for admins except the audit.** Admin campaign creation gains no team-domain dependency. The stored `ad_campaigns.seller_id` semantics stay out of scope.
7. **Fail closed, no compensation.** If the insert fails after the audit write, the error is returned as today (Internal) and the trail has one extra entry; never an action without an entry.
8. **Principal.** `service-team-promotion`, type `service`, scope `audit.write`, set by a dedicated unary interceptor that overwrites outgoing metadata (the domain client's forward-the-caller interceptor is not reused).
9. **Event emission** (`FlashSaleChanged`, `VoucherChanged`) stays after the insert, so audit-down emits nothing downstream.

## Clarifications

### Round 1 — 2026-10-15

- Q1 (permission): uniform rule, every campaign/ad-campaign creation by an `admin` caller is audited with no owner lookup; no team-domain `GetListing` on the admin path, so no Unavailable/InvalidArgument behaviour change. `seller_id` metadata uses the already-stored seller id for ad campaigns; flash sale records `listing_id` because no seller is available without a lookup. Seller paths unaudited. Stored `ad_campaigns.seller_id` semantics out of scope.
- Scenario titles are prefixed "Promotion audit: ..." where they would collide with other changes.
- E2E uses the audit-query-filters helpers `assert_one_event`, `assert_no_event`, `stop_audit(world)`, `start_audit(world)`; audit-down scenarios are tagged `@destructive`. `ListSponsoredSlots` is gateway-routed (`SponsoredForwarder` in `team-gateway/internal/edge/promotion.go`, registered in `server.go`), so the ad-campaign audit-down check uses it.
