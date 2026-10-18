Depends on `pah-contract` (catalogue action `voucher.admin_create`, vendored audit proto comment in team-promotion) and on the audit client, `AuditWriter` and compose `UPSTREAM_AUDIT_ADDR` delivered by `promotion-admin-audit`.

## Code track (team-promotion)

- [ ] 1. Service. Write-set: `team-promotion/internal/service/voucher.go`, `internal/service/voucher_test.go`. Add `AuditAdmin bool` to `CreateVoucherParams`; when `Scope == SHOP && AuditAdmin`, call `writeAudit` (action `voucher.admin_create`, metadata `discount_type`, `value`, `seller_id`) before `vouchers.Create`; keep the PLATFORM branch as is. Scenarios: Shop voucher audit: audit precedes the insert, Shop voucher audit: audit not configured, Shop voucher audit: audit error blocks the voucher, Shop voucher audit: platform scope keeps its action.
- [ ] 2. Handler. Write-set: `team-promotion/internal/handler/voucher.go`, `internal/handler/audit_test.go`. In the SHOP branch set `AuditAdmin: interceptor.IsAdmin(principal)`; pass `Unavailable` through unchanged. Scenarios: Shop voucher audit: admin shop voucher is audited by the handler with the admin as actor, Shop voucher audit: seller path needs no audit writer, Shop voucher audit: refused callers write nothing, Shop voucher audit: invalid request writes nothing.
- [ ] 3. Docs. Write-set: `team-promotion/README.md` (audited actions list gains `voucher.admin_create`). Scenarios: Shop voucher audit: admin creates a shop voucher.

## E2E track (platform-e2e)

- [ ] 4. Feature and steps. Write-set: `platform-e2e/tests/e2e/features/promo/promotion_shop_voucher_audit.feature`, `platform-e2e/tests/e2e/step_definitions/test_promotion_shop_voucher_audit.py` (reuse `support/audit_support.py` `assert_one_event`, `assert_no_event`, `stop_audit`, `start_audit`; restart audit in teardown). Scenarios in the feature:
  - "An admin creates a shop voucher with an audit event"
  - "A seller's own shop voucher is not audited" (`@destructive`, audit stopped)
  - "Admin shop voucher creation is refused while audit is down" (`@destructive`)

  Scenarios: Shop voucher audit: admin creates a shop voucher, Shop voucher audit: seller's own shop voucher is not audited, Shop voucher audit: audit down blocks an admin shop voucher.
- [ ] 5. `team-promotion/FEATURES.yaml` is written by the e2e-author, not this slice; existing `promotion.admin-audit-*` entries stay. Scenarios: Shop voucher audit: admin creates a shop voucher.
