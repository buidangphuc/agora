# promotion-shop-voucher-audit Specification

## Purpose
An admin creating a SHOP voucher writes one voucher.admin_create audit event first and fails closed when audit is unavailable.

## Requirements

### Requirement: Admin-created shop vouchers are audited and fail closed

`PromotionService.CreateVoucher` called with `VOUCHER_SCOPE_SHOP` by a caller holding the `admin` scope SHALL, after authentication, authorization and request validation, write exactly one team-audit event before the voucher row is inserted. The event SHALL have `action=voucher.admin_create`, `target_type=voucher`, `target_id` equal to the voucher code, `actor_id` equal to the admin's user id, and `metadata` keys `discount_type` (enum name), `value` (discount value in decimal) and `seller_id` (the id the voucher is stored under). IF the audit write fails, times out or `UPSTREAM_AUDIT_ADDR` is unset, THEN the RPC SHALL return `Unavailable`, SHALL insert no voucher and SHALL emit no `VoucherChanged`. A caller holding `listing.write` without `admin` creating a SHOP voucher SHALL write no audit event and SHALL NOT depend on team-audit. PLATFORM and UNSPECIFIED scope behaviour (`voucher.platform_create`) SHALL be unchanged.

#### Scenario: Shop voucher audit: admin creates a shop voucher
- **WHEN** an admin creates a SHOP voucher with code "SHOP-ADM10", discount type percent, value 10 through the gateway
- **THEN** the call returns OK, the voucher exists, and QueryAuditLog (admin) for target_type `voucher` and target_id "SHOP-ADM10" returns exactly one event with action `voucher.admin_create`, the admin's id as actor_id, and metadata `discount_type`=`DISCOUNT_TYPE_PERCENT`, `value`=`10`, `seller_id`=the admin's id
- **VERIFIED BY**: platform-e2e `features/promo/promotion_shop_voucher_audit.feature` "An admin creates a shop voucher with an audit event"

#### Scenario: Shop voucher audit: seller's own shop voucher is not audited
- **WHEN** a seller creates a SHOP voucher with code "SHOP-SELL10" while team-audit is stopped
- **THEN** the call returns OK and QueryAuditLog returns zero events for target_id "SHOP-SELL10" once team-audit is restarted
- **VERIFIED BY**: platform-e2e `features/promo/promotion_shop_voucher_audit.feature` "A seller's own shop voucher is not audited"

#### Scenario: Shop voucher audit: audit precedes the insert
- **WHEN** an admin creates a SHOP voucher
- **THEN** the recorded call order is audit write, then repository insert, then emit
- **VERIFIED BY**: team-promotion `internal/service` `TestCreateVoucherShopAdminAuditBeforeInsert`

#### Scenario: Shop voucher audit: audit down blocks an admin shop voucher
- **WHEN** team-audit is stopped and an admin creates a SHOP voucher with code "SHOP-DOWN"
- **THEN** the call returns `unavailable` and GetVoucher for "SHOP-DOWN" returns not found
- **VERIFIED BY**: platform-e2e `features/promo/promotion_shop_voucher_audit.feature` "Admin shop voucher creation is refused while audit is down"

#### Scenario: Shop voucher audit: audit not configured
- **WHEN** an admin creates a SHOP voucher and the AuditWriter is nil (`UPSTREAM_AUDIT_ADDR` unset)
- **THEN** the call returns `Unavailable`, the repository holds no voucher and no `VoucherChanged` is emitted
- **VERIFIED BY**: team-promotion `internal/service` `TestCreateVoucherShopAdminNilAuditUnavailable`

#### Scenario: Shop voucher audit: audit error blocks the voucher
- **WHEN** the audit write returns an error or exceeds `UPSTREAM_CALL_TIMEOUT_SECONDS` (2 s)
- **THEN** the call returns `Unavailable` within 2.5 s and no voucher row exists
- **VERIFIED BY**: team-promotion `internal/service` `TestCreateVoucherShopAdminAuditErrorFailsClosed`

#### Scenario: Shop voucher audit: seller path needs no audit writer
- **WHEN** a seller (`listing.write`, not admin) creates a SHOP voucher and the AuditWriter is nil
- **THEN** the call returns OK and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/handler` `TestCreateVoucherShopSellerWritesNoAudit`

#### Scenario: Shop voucher audit: admin shop voucher is audited by the handler with the admin as actor
- **WHEN** a principal with scope `admin` and id "admin-1" calls CreateVoucher with SHOP scope
- **THEN** the AuditWriter receives exactly one event with action `voucher.admin_create` and actor_id "admin-1"
- **VERIFIED BY**: team-promotion `internal/handler` `TestCreateVoucherShopAdminAudited`

#### Scenario: Shop voucher audit: refused callers write nothing
- **WHEN** a buyer (no `listing.write`, no `admin`) calls CreateVoucher with SHOP scope, and an anonymous caller does the same
- **THEN** the calls return `permission_denied` and `unauthenticated` respectively and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/handler` `TestCreateVoucherShopRefusedWritesNoAudit`

#### Scenario: Shop voucher audit: invalid request writes nothing
- **WHEN** an admin calls CreateVoucher with SHOP scope and an empty code
- **THEN** the call returns `InvalidArgument` and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/handler` `TestCreateVoucherShopAdminInvalidWritesNoAudit`

#### Scenario: Shop voucher audit: platform scope keeps its action
- **WHEN** an admin creates a PLATFORM voucher after this change
- **THEN** exactly one event with action `voucher.platform_create` is written and none with `voucher.admin_create`
- **VERIFIED BY**: team-promotion `internal/service` `TestCreateVoucherPlatformStillPlatformAction`
