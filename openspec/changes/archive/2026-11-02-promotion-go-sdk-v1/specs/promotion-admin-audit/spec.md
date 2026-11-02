## REMOVED Requirements

### Requirement: Platform voucher creation is audited and fails closed

**Reason**: the requirement allowed a mode with no audit writer (UPSTREAM_AUDIT_ADDR unset) and bounded the write by UPSTREAM_CALL_TIMEOUT_SECONDS; the go-sdk migration deletes both.

**Migration**: replaced by the requirement "Platform voucher creation is audited through the gate and fails closed" below, which keeps every scenario that still applies and has no mode without the audit writer.

### Requirement: Audit upstream is configured and wired only when set

**Reason**: the requirement allowed a mode with no audit writer (UPSTREAM_AUDIT_ADDR unset) and bounded the write by UPSTREAM_CALL_TIMEOUT_SECONDS; the go-sdk migration deletes both.

**Migration**: replaced by the requirement "Audit upstream is required and written through auditx" below, which keeps every scenario that still applies and has no mode without the audit writer.

## ADDED Requirements

### Requirement: Platform voucher creation is audited through the gate and fails closed

`VoucherService.CreateVoucher` called with `VOUCHER_SCOPE_PLATFORM` or `VOUCHER_SCOPE_UNSPECIFIED` SHALL, after the caller is authenticated, holds `admin` and the request is valid, write one team-audit event before the voucher row is inserted. The event SHALL have `action=voucher.platform_create`, `target_type=voucher`, `target_id` equal to the voucher code, `actor_id` equal to the admin's user id, and `metadata` keys `discount_type` (enum name) and `value` (discount value in decimal). IF the audit write fails, or times out, THEN the RPC SHALL return `Unavailable` and SHALL insert no voucher. Shop-scoped vouchers created by a seller SHALL write no audit event and SHALL NOT depend on team-audit.

#### Scenario: Admin creates a platform voucher
- **WHEN** an admin creates a PLATFORM voucher with code "PLAT10", discount type percent, discount value 10 through the gateway
- **THEN** the call returns OK, the voucher "PLAT10" exists, and QueryAuditLog (admin) for target_type `voucher` and target_id "PLAT10" returns exactly one event with action `voucher.platform_create`, the admin's id as actor_id, and metadata `discount_type`=`DISCOUNT_TYPE_PERCENT`, `value`=`10`
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "An admin creates a platform voucher with an audit event"

#### Scenario: Unspecified scope is audited like platform
- **WHEN** an admin calls CreateVoucher with scope unspecified
- **THEN** exactly one `voucher.platform_create` event is written before the insert
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateVoucherUnspecifiedScopeAudited`

#### Scenario: Promotion audit precedes the voucher insert
- **WHEN** an admin creates a platform voucher
- **THEN** the recorded call order is audit write, then repository insert
- **VERIFIED BY**: team-promotion `internal/voucher` `TestCreateVoucherAuditBeforeInsert`

#### Scenario: Audit down blocks a platform voucher
- **WHEN** team-audit is stopped and an admin creates a PLATFORM voucher with code "PLAT-DOWN"
- **THEN** the call returns `unavailable` and GetVoucher for "PLAT-DOWN" returns not found
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "Platform voucher creation is refused while audit is down"

#### Scenario: Promotion audit timeout blocks the voucher
- **WHEN** the audit call exceeds the 2 s `auditx` write bound
- **THEN** the call returns `Unavailable` within 2.5 s and no voucher row exists
- **VERIFIED BY**: team-promotion `internal/upstream` `TestAuditClientTimeoutBounded` and `internal/voucher` `TestCreateVoucherAuditErrorFailsClosed`

#### Scenario: Shop voucher by a seller writes no event
- **WHEN** a seller creates a SHOP voucher with team-audit stopped
- **THEN** the call returns OK and QueryAuditLog returns no `voucher.platform_create` event for that code
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "A seller's own promotions are not audited"

#### Scenario: Non-admin cannot create a platform voucher and nothing is audited
- **WHEN** a seller or a buyer calls CreateVoucher with PLATFORM scope, and an anonymous caller does the same
- **THEN** the calls return `permission_denied`, `permission_denied` and `unauthenticated` respectively, and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateVoucherPlatformDeniedWritesNoAudit`

#### Scenario: Invalid platform voucher writes no event
- **WHEN** an admin calls CreateVoucher with an empty code
- **THEN** the call returns `InvalidArgument` and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateVoucherInvalidWritesNoAudit`

#### Scenario: Promotion audit is called as the service principal
- **WHEN** the audit client sends WriteAuditEvent
- **THEN** the outgoing metadata principal is `service-team-promotion`, type `service`, scopes `audit.write`, and the admin's own principal metadata is absent
- **VERIFIED BY**: team-promotion `internal/upstream` `TestAuditClientUsesServicePrincipal`

### Requirement: Audit upstream is required and written through auditx

team-promotion SHALL read `UPSTREAM_AUDIT_ADDR` through the SDK `config.Audit` group, SHALL require it in every `ENV` (no mode without an audit writer), and SHALL build its audit client with `auditx.Dial` as the service principal `service-team-promotion`. IF `UPSTREAM_AUDIT_ADDR` is empty, THEN settings load SHALL fail. Every audited action SHALL run through `auditx.Gate`, which writes the event first and refuses the action with `Unavailable` and the message `audit trail unavailable` when the write fails or times out; seller paths SHALL be unaffected by audit availability. The local compose stack SHALL set `UPSTREAM_AUDIT_ADDR` to `team-audit-svc:50066` for team-promotion.

#### Scenario: Env key is declared
- **WHEN** the config drift check runs
- **THEN** `UPSTREAM_AUDIT_ADDR` is present in both `internal/config` and `.env.example`
- **VERIFIED BY**: team-promotion `internal/config` `TestEnvExampleInSync`

#### Scenario: An empty audit address is refused in every ENV
- **WHEN** settings load under `ENV=local` and again under `ENV=prod` with `UPSTREAM_AUDIT_ADDR` empty
- **THEN** each load returns an error naming `UPSTREAM_AUDIT_ADDR`
- **VERIFIED BY**: team-promotion `internal/config` `TestLoadRequiresDatabaseURLAndUpstreamAddrs`

#### Scenario: A failing audit writer refuses the audited action with one message
- **WHEN** the audit writer fails every write and an admin creates a PLATFORM voucher, an admin flash-sale campaign and an admin ad campaign
- **THEN** each call returns `Unavailable` with the message `audit trail unavailable` and no row is stored
- **VERIFIED BY**: team-promotion `internal/voucher` `TestCreateVoucherAuditDownUnavailable`

#### Scenario: A seller is denied a platform voucher while audit is down
- **WHEN** the audit writer fails every write and a seller calls CreateVoucher with PLATFORM scope
- **THEN** the call returns `PermissionDenied` (not `Unavailable`) and the audit writer is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSellerCannotCreatePlatformVoucher`

#### Scenario: Seller paths unaffected with audit down
- **WHEN** the audit writer fails every write and a seller creates a SHOP voucher, a flash-sale campaign and an ad campaign on their own listing
- **THEN** all three calls return OK
- **VERIFIED BY**: team-promotion `internal/rpc` `TestSellerPathsIgnoreAuditAvailability`

## MODIFIED Requirements

### Requirement: Admin-created flash-sale campaigns are audited

`FlashSaleService.CreateCampaign` called by a caller holding `admin` SHALL, after authn/authz and validation and with no team-domain lookup, write one team-audit event before the insert. The event SHALL have `action=flash_sale.admin_create`, `target_type=flash_sale`, `target_id` equal to the campaign id generated before the insert (the id returned in the response), `actor_id` equal to the admin's user id, and `metadata` key `listing_id` (a flash-sale campaign has no seller field and the service resolves no owner for admins, so the listing id replaces `seller_id`). IF the audit write fails, or times out, THEN the RPC SHALL return `Unavailable`, SHALL insert no campaign and SHALL emit no `FlashSaleChanged`. A non-admin seller creating a campaign SHALL write no event and SHALL NOT depend on team-audit.

#### Scenario: Admin creates a flash sale with an audit event
- **WHEN** an admin creates a flash-sale campaign on a listing owned by seller S through the gateway
- **THEN** the call returns OK and QueryAuditLog for target_type `flash_sale` and target_id equal to the returned campaign id returns exactly one event with action `flash_sale.admin_create`, the admin's id as actor_id and metadata `listing_id` equal to the listing
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "An admin creates a flash sale with an audit event"

#### Scenario: Seller creates a flash sale on their own listing
- **WHEN** seller S creates a flash-sale campaign on their own listing, including while team-audit is stopped
- **THEN** the call returns OK and QueryAuditLog returns no `flash_sale.admin_create` event for the returned campaign id
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "A seller's own promotions are not audited"

#### Scenario: Admin flash sale makes no owner lookup
- **WHEN** an admin creates a flash-sale campaign and the listing client returns an error
- **THEN** the call returns OK and exactly one audit event is written
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignAdminNoOwnerLookup`

#### Scenario: Audit down blocks an admin flash sale
- **WHEN** team-audit is stopped and an admin creates a flash-sale campaign
- **THEN** the call returns `unavailable`, and GetActiveFlashSale for that listing reports `active` false
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "Admin promotions are refused while audit is down"

#### Scenario: Promotion audit precedes the campaign insert and carries the final id
- **WHEN** an admin creates a flash-sale campaign
- **THEN** the recorded call order is audit write, then repository insert, and the audit target_id equals the stored campaign id
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignAuditBeforeInsertSameID`

#### Scenario: Promotion audit failure emits nothing for a flash sale
- **WHEN** the audit call errors or times out for an admin campaign
- **THEN** the call returns `Unavailable`, the repository holds no campaign and `FlashSaleChanged` is not emitted
- **VERIFIED BY**: team-promotion `internal/flashsale` `TestCreateCampaignAuditFailureFailsClosed`

#### Scenario: Invalid campaign writes no event
- **WHEN** an admin calls CreateCampaign with an empty listing_id
- **THEN** the call returns `InvalidArgument` and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateCampaignInvalidWritesNoAudit`

#### Scenario: Refused callers write no flash-sale event
- **WHEN** a buyer calls CreateCampaign, an anonymous caller calls it, and a seller calls it on another seller's listing
- **THEN** the calls return `permission_denied`, `unauthenticated` and `permission_denied` respectively and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateCampaignDeniedWritesNoAudit`

### Requirement: Admin-created ad campaigns are audited

`SponsoredService.CreateAdCampaign` called by a caller holding `admin` SHALL, after authn/authz and budget/bid validation and with no team-domain lookup, write one team-audit event before the insert. The event SHALL have `action=ad_campaign.admin_create`, `target_type=ad_campaign`, `target_id` equal to the campaign id generated before the insert, `actor_id` equal to the admin's user id, and `metadata` keys `seller_id` (the seller id the service stores on the campaign, which is the calling principal's id as today), `listing_id` and `budget` (decimal). IF the audit write fails, or times out, THEN the RPC SHALL return `Unavailable` and SHALL insert no campaign. A non-admin seller creating an ad campaign SHALL write no event.

#### Scenario: Admin creates an ad campaign with an audit event
- **WHEN** an admin creates an ad campaign with budget 50000 and bid 1000 on a listing through the gateway
- **THEN** the call returns OK and QueryAuditLog for target_type `ad_campaign` and the returned campaign id returns exactly one event with action `ad_campaign.admin_create`, the admin's id as actor_id and metadata `seller_id` equal to the returned campaign's seller_id, `listing_id` equal to the listing, `budget`=`50000`
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "An admin creates an ad campaign with an audit event"

#### Scenario: Seller creates an ad campaign on their own listing
- **WHEN** seller S creates an ad campaign on their own listing, including while team-audit is stopped
- **THEN** the call returns OK and QueryAuditLog returns no `ad_campaign.admin_create` event for the returned campaign id
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "A seller's own promotions are not audited"

#### Scenario: Audit down blocks an admin ad campaign
- **WHEN** team-audit is stopped and an admin creates an ad campaign on a listing
- **THEN** the call returns `unavailable` and ListSponsoredSlots (routed by the gateway, `SponsoredForwarder`) does not contain that listing
- **VERIFIED BY**: platform-e2e `features/promo/promotion_admin_audit.feature` "Admin promotions are refused while audit is down"

#### Scenario: Promotion audit precedes the ad campaign insert and carries the final id
- **WHEN** an admin creates an ad campaign
- **THEN** the recorded call order is audit write, then repository insert, and the audit target_id equals the stored campaign id
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignAuditBeforeInsertSameID`

#### Scenario: Promotion audit failure leaves no ad campaign
- **WHEN** the audit call errors or times out for an admin ad campaign
- **THEN** the call returns `Unavailable` and the repository holds no campaign
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignAuditFailureFailsClosed`

#### Scenario: Budget boundary is validated before the audit
- **WHEN** an admin creates an ad campaign with budget equal to the configured maximum, and another with budget one above the maximum
- **THEN** the first is audited and created, the second returns `InvalidArgument` with no audit call
- **VERIFIED BY**: team-promotion `internal/sponsored` `TestCreateAdCampaignBudgetBoundaryAudit`

#### Scenario: Refused callers write no ad-campaign event
- **WHEN** a caller without `listing.write` calls CreateAdCampaign, and a seller creates one on another seller's listing
- **THEN** both calls return `permission_denied` and the AuditWriter is never called
- **VERIFIED BY**: team-promotion `internal/rpc` `TestCreateAdCampaignDeniedWritesNoAudit`
