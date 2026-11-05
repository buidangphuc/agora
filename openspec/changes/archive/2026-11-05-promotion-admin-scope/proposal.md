## Why

team-promotion recognises an admin by the generic `admin` role marker. Every admin-only promotion RPC
(`DeactivateVoucher`, `UpdateVoucher`, `ListCampaigns`, `CancelCampaign`, `UpdateCampaign`, platform
`CreateVoucher`) and every admin branch of a mixed RPC (shop `CreateVoucher`, flash-sale `CreateCampaign`,
`CreateAdCampaign`) is opened by that marker alone, and the gateway gates the five admin procedures on `admin`.
team-order already moved to a domain scope (`order.admin`, change authz-residuals-2); promotion is the next domain
that changes money-relevant state on a seller's behalf and needs the same domain scope.

## What Changes

- **team-identity**: the admin role is issued a new scope `promotion.admin`, next to `admin` and `order.admin`.
  Buyer, seller and service principals never get it.
- **team-promotion**:
  - `DeactivateVoucher`, `UpdateVoucher`, `ListCampaigns`, `CancelCampaign`, `UpdateCampaign` and `CreateVoucher`
    with PLATFORM or UNSPECIFIED scope require both `admin` and `promotion.admin`.
  - Mixed RPCs (shop `CreateVoucher`, `CreateCampaign`, `CreateAdCampaign`) take the admin branch (no ownership
    check, idempotency key on `CreateCampaign`, admin audit event) only when the caller holds `promotion.admin`.
    A caller holding `admin` without `promotion.admin` is treated like any non-admin caller: the seller path when it
    holds `listing.write`, otherwise `permission_denied`.
  - Audit behaviour on the admin paths (audit first, fail closed with `AUDIT_UNAVAILABLE`) is unchanged.
- **team-gateway**: `adminProcedures` maps the five promotion admin procedures to `promotion.admin` instead of `admin`.
- **platform-e2e**: `security/promotion_admin_scope.feature` for the token, edge and service rules.
- **Docs**: `AGENTS.md` rule 6 and `.claude/agents/auth-scope-reviewer.md` name `promotion.admin` next to `order.admin`.

Repos touched: team-identity, team-promotion, team-gateway, platform-e2e, workspace docs. **No proto change.**

**BREAKING:** an admin token issued before this change has no `promotion.admin`; until the admin signs in again the
five admin procedures answer 403 at the gateway, platform voucher creation answers 403 and the mixed RPCs take the
seller path (tokens live at most `JWT_TTL`).

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `auth`: the admin role is issued `promotion.admin`.
- `edge-route-policy`: the five promotion admin procedures are gated on `promotion.admin`.
- `promotion-access-control`: admin-only promotion RPCs need `admin` and `promotion.admin`; mixed RPCs take the
  admin branch only with `promotion.admin`.
- `promotion-admin-audit`: the flash-sale, ad-campaign and platform-voucher admin audit applies to the
  `promotion.admin` caller.
- `promotion-shop-voucher-audit`: the shop-voucher admin audit applies to the `promotion.admin` caller.
- `audit-query-contract`: the workspace audit-first rule names `promotion.admin` for team-promotion.

## Non-goals

- Any proto change. The `audit.proto` `WriteAuditEventRequest` comment and the promotion RPC comments still name
  `admin`; re-vendoring the audit proto to name `promotion.admin` is a separate contract change.
- team-frontend-admin scope checks (the admin app keeps its `admin` gate; the backend is authoritative).
- New scopes for other domains (listing, engagement, verification, analytics).
- Changing which RPCs are audited or the audit event shape.

## Impact

- Code: `team-identity/internal/authz/scopes.go`; `team-promotion/internal/rpc/{common,voucher,flashsale,sponsored}.go`;
  `team-gateway/internal/edge/policy.go`.
- Tests: identity authz tests, promotion rpc tests, gateway policy tests, new e2e feature.
- FEATURES.yaml: team-identity, team-promotion, team-gateway.
- Rollout: rebuild team-identity, team-promotion and team-gateway together; admins sign in again.

## Non-functional requirements

- No added call or latency: the checks are in-memory scope lookups on the already resolved principal.
- Availability unchanged: no new dependency; audit fail-closed behaviour is unchanged.

## Data classification

none. The change adds a scope string to admin tokens and changes authorization decisions; no new personal data is
collected, stored, logged or sent.

## Threat model

| Asset | Threat (STRIDE) | Mitigation |
|---|---|---|
| Platform vouchers and flash-sale campaigns | Elevation of privilege: any token carrying the generic `admin` marker changes promotion state | Admin-only RPCs need `admin` AND `promotion.admin`; gateway gates the five procedures on `promotion.admin` |
| Seller listings (ad and flash-sale campaigns) | Elevation of privilege: bare `admin` skips the listing-ownership check | Admin branch only with `promotion.admin`; otherwise the seller ownership check applies |
| Audit trail | Repudiation: admin action taken without an audit event | Admin branch and its audit write are one path keyed on `promotion.admin`; fail closed unchanged |
| Role table | Elevation of privilege: buyer or seller role gains `promotion.admin` | Identity unit test pins `promotion.admin` to the admin role only; e2e checks buyer and seller tokens |
| Old admin tokens | Denial of service: admins lose access until re-login | Documented BREAKING; tokens expire within `JWT_TTL` |
