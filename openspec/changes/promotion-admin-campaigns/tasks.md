# Tasks: promotion-admin-campaigns

## Code track
- [ ] 1. Store: add `cancelled_at` to flash_sale_campaigns, idempotency table `(actor_id, key)` unique with payload hash and `created_at`, 24 h purge, `List(statuses, offset, limit)` ordered by starts_at desc, atomic cancel and update-if-scheduled (scenarios: List filters by status, Paging boundary, Concurrent duplicates create one campaign, Key is scoped per admin and expires)
- [ ] 2. flashsale service: derived status; `ListCampaigns`; `flash_sale_disabled` on both lists (List filters by status, Status derivation at window edges, Kill-switch off is reported)
- [ ] 3. flashsale service: `CancelCampaign` with audit-first, no-op when cancelled, FAILED_PRECONDITION when ended; `ReserveFlashSaleStock` refuses cancelled with `not_active`; commit/release of existing holds unchanged (all Cancel scenarios)
- [ ] 4. flashsale service: `UpdateCampaign` scheduled only, audit-first, shared validation (all Edit scenarios)
- [ ] 5. flashsale service: idempotency lookup BEFORE validation and listing lookup (hit returns current stored state); ErrorInfo domain promotion.platform for AUDIT_UNAVAILABLE and LISTING_UNAVAILABLE; shared validation using `ListingService.GetListing` on seller and admin paths; LISTING_UNAVAILABLE ErrorInfo; idempotent admin create with payload hash (all validation and idempotency scenarios)
- [ ] 6. rpc handlers `ListCampaigns`, `CancelCampaign`, `UpdateCampaign`, `CreateCampaign` key check; `requireAdmin` for the new RPCs (Non-admin cannot list all campaigns, Seller cannot cancel, Seller cannot edit, Missing or malformed key, Seller create needs no key)
- [ ] 7. team-gateway `internal/edge/promotion.go` and scope table: expose three RPCs with admin scope (Non-admin cannot list all campaigns); keep ErrorInfo reason pass-through
- [ ] 8. team-frontend-admin `src/features/campaigns` + `src/app/admin/campaigns` (+ `new`): list, filter, paging, cancel, edit, retry-safe create, messages, banner (all Admin campaign screens scenarios); reuse the shared error mapping from promotion-admin-vouchers, extended for PROMOTION_UNAVAILABLE
- [ ] 9. Remove rows G12, G13, G14 from `fe-platform/docs/gaps.md`

- [ ] 12. Bench `BenchmarkListCampaignsPage50` (informational p95)

## E2E track
- [ ] 10. platform-e2e `features/promo/promotion_admin_campaigns.feature` with the scenarios citing it above (list filter, kill-switch both lists, non-admin refused, cancel keeps held and sold, cancel audit down, edit scheduled, edit started refused, price at or above listing, unknown listing, listing outage, retry same key, same key different payload, UI cancel, UI retry)
- [ ] 11. e2e page object for admin campaigns screens; relies on `wait_streamed` in BasePage
