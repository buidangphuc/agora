## ADDED Requirements

### Requirement: Promotion admin-only RPCs require admin and promotion.admin

team-promotion SHALL serve `DeactivateVoucher`, `UpdateVoucher`, `ListCampaigns`, `CancelCampaign`, `UpdateCampaign`
and `CreateVoucher` with `VOUCHER_SCOPE_PLATFORM` or `VOUCHER_SCOPE_UNSPECIFIED` only to a principal holding both the
`admin` scope and the `promotion.admin` scope. Any other authenticated caller (a buyer, a seller, a SERVICE principal,
a token with `admin` but not `promotion.admin`, a token with `promotion.admin` but not `admin`) SHALL receive
`permission_denied`, and the call SHALL change no voucher or campaign and write no audit event. An anonymous caller
SHALL receive `unauthenticated`.

#### Scenario: Promotion admin scope: the admin deactivates a platform voucher

- **WHEN** the seeded admin creates a PLATFORM voucher and then calls `DeactivateVoucher` on it through the gateway
- **THEN** the call succeeds and the returned voucher is inactive

#### Scenario: Promotion admin scope: promotion.admin without admin is denied by the service

- **WHEN** a caller whose token carries `promotion.admin` but not `admin` calls `DeactivateVoucher` on an active PLATFORM voucher through the gateway
- **THEN** the call fails with `permission_denied` and the voucher is still active

#### Scenario: Promotion admin scope: admin without promotion.admin cannot create a platform voucher

- **WHEN** a caller whose token carries `admin` and `listing.write` but not `promotion.admin` calls `CreateVoucher` with PLATFORM scope through the gateway
- **THEN** the call fails with `permission_denied` and `GetVoucher` for that code returns `not_found`

#### Scenario: Promotion admin scope: sellers and anonymous callers are refused

- **WHEN** a seller calls `CancelCampaign` and a request without a token calls `UpdateVoucher` through the gateway
- **THEN** the gateway answers HTTP 403 to the seller and HTTP 401 to the anonymous request

### Requirement: Mixed promotion RPCs take the admin path only with promotion.admin

`CreateVoucher` with `VOUCHER_SCOPE_SHOP`, `FlashSaleService.CreateCampaign` and `SponsoredService.CreateAdCampaign`
SHALL take their admin branch (no listing-ownership check, the `idempotency_key` rule on `CreateCampaign`, and the
admin audit event written first) only when the caller holds `promotion.admin`. A caller holding `admin` without
`promotion.admin` SHALL be treated as a non-admin caller: when it holds `listing.write` it SHALL take the seller path
(ownership check, no audit event), otherwise it SHALL receive `permission_denied`.

#### Scenario: Promotion admin scope: admin without promotion.admin creates a shop voucher as a seller

- **WHEN** a caller whose token carries `admin` and `listing.write` but not `promotion.admin` creates a SHOP voucher through the gateway
- **THEN** the call succeeds, the voucher's `seller_id` is the caller's id, and `QueryAuditLog` returns no `voucher.admin_create` event for that code

#### Scenario: Promotion admin scope: admin without promotion.admin cannot run a flash sale on another seller's listing

- **WHEN** a caller whose token carries `admin` and `listing.write` but not `promotion.admin` calls `CreateCampaign` for a listing owned by seller S through the gateway
- **THEN** the call fails with `permission_denied` and `ListCampaigns` (seeded admin) returns no campaign for that listing

#### Scenario: Promotion admin scope: admin without promotion.admin cannot advertise another seller's listing

- **WHEN** a caller whose token carries `admin` and `listing.write` but not `promotion.admin` calls `CreateAdCampaign` for a listing owned by seller S through the gateway
- **THEN** the call fails with `permission_denied`

#### Scenario: Promotion admin scope: admin without promotion.admin or listing.write is denied a shop voucher

- **WHEN** a caller whose token carries only `admin` calls `CreateVoucher` with SHOP scope through the gateway
- **THEN** the call fails with `permission_denied`
