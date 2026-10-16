## Why

Three team-promotion mutations succeed only because the caller is an admin, and none leaves a trail: `CreateVoucher` with PLATFORM (or unspecified) scope (`internal/handler/voucher.go`, `RequireAdmin` branch), `CreateCampaign` (flash sale) and `CreateAdCampaign` when the caller holds `admin` (`IsAdmin` skips `requireListingOwner` in `internal/service/flashsale.go` and `sponsored.go`). A platform voucher is platform-funded discount money, and an admin-made campaign changes what a seller's listing sells for or promotes. The initiative (`plans/q3-audit-log/00-spec.md`, Locked 2026-10-15) requires each such action to write one team-audit event before the change and to fail closed if the event cannot be written. This slice covers the three promotion rows of the catalogue: `voucher.platform_create`, `flash_sale.admin_create`, `ad_campaign.admin_create`.

initiative: q3-audit-log

## What Changes

- **team-promotion, new `internal/upstream/audit.go`** (mirrors `team-engagement/internal/upstream/audit.go`, in the style of `upstream/domain.go`):
  - `AuditClient`, `NewAuditClient(addr, timeout)` returns `(nil, nil)` when `UPSTREAM_AUDIT_ADDR` is empty; one generic write method (actor id, action, target type, target id, metadata map).
  - Calls `AuditService.WriteAuditEvent` as service principal `service-team-promotion`, type `service`, scope `audit.write`, through its own interceptor that replaces any outgoing principal. The admin's principal is never forwarded and `actor_id` is the admin's user id from the verified Principal.
  - Bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (default 2 s), no retry.
  - Config `UPSTREAM_AUDIT_ADDR` (default empty) in `internal/config`, `.env.example`, README; wiring in `cmd/server/main.go` only when the client is non-nil.
  - Small `AuditWriter` interface injected into `VoucherService`, `FlashSaleService`, `SponsoredService` (`WithAudit`), so unit tests use a fake.
  - Audit proto copy regenerated (`make proto`) from the vendored `platform/audit/v1` copy placed by the contract slice `audit-query-filters`.
- **`CreateVoucher`**: after `RequirePrincipal`, the code check and the scope/role check, and before the insert, the PLATFORM and UNSPECIFIED branches write `voucher.platform_create` (target_type `voucher`, target_id = the voucher code, metadata `discount_type`, `value`). SHOP scope (seller) is never audited. The code is the target because it is the client-supplied unique key and exists before creation; the surrogate row id is generated inside the repository.
- **`CreateCampaign` and `CreateAdCampaign`**: uniform rule, every call by a caller holding `admin` is audited, with no owner lookup and no new upstream call. After validation and before the insert, write `flash_sale.admin_create` (metadata `listing_id`) or `ad_campaign.admin_create` (metadata `seller_id` as stored, `listing_id`, `budget`). Non-admin sellers keep the existing `requireListingOwner` path: no audit, no added call.
- **Campaign target id**: ids are generated inside the repository `Create` today (`newID()` when `ID == ""`). The campaign id is therefore generated in the service before the audit write (same 128-bit hex scheme, unexported `repository.newID` is not reachable, so a service-local helper is added) and passed in the entity; all three repository `Create` methods already keep a preset ID. Target id = campaign id. See `design.md`.
- **Audit write failure, timeout, nil client** (`UPSTREAM_AUDIT_ADDR` unset): the RPC returns `Unavailable`; no row is inserted and `FlashSaleChanged` is not emitted.
- **platform-e2e**: new feature `features/promo/promotion_admin_audit.feature` + step bindings, one scenario per action, seller path (no event, unaffected by audit-down), and audit-down per action. Imports `tests/e2e/support/audit_support.py` (written by `audit-query-filters`). `UPSTREAM_AUDIT_ADDR=team-audit-svc:50066` for team-promotion in `docker-compose.services.yaml` is added by the contract slice, not here.
- Not changed: seller `CreateVoucher`, `ValidateAndReserve`/commit/release, subscriptions, the ad campaign stored `seller_id` (still the calling principal), proto, gateway policy, team-audit, team-identity.

## NFR

- Admin paths add exactly one team-audit call, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (<= 2 s, also capped by the inbound deadline), never retried.
- Owner/seller paths: no added call and no added latency; they do not depend on team-audit availability.
- Availability: team-audit down blocks only the three admin-path creations (fail closed, `Unavailable`). No new team-domain dependency.
- Limits: unchanged request limits; audit metadata values are ids, enum names and integers only.

## Data classification

| Field | Class | Purpose | Basis | Retention |
|---|---|---|---|---|
| audit `actor_id` (admin user id) | personal (staff identifier) | accountability of privileged actions | legal obligation / legitimate interest (Decree 13/2023 processing records) | team-audit policy (immutable) |
| audit `target_id` (voucher code, campaign id) | business data | identify what was touched | same | same |
| metadata `seller_id` (stored on the ad campaign) and `listing_id` | personal-light (account identifier) | which seller/listing was acted for | same | same |
| metadata `discount_type`, `value`, `budget` | business data | size of the discount or spend | same | same |

No names, contact data or free text enter the event. Nothing new is stored in team-promotion tables; nothing new goes to analytics or logs beyond ids already logged.

## Threat model

- **Assets:** platform-funded vouchers, flash-sale pricing, sponsored placement, the audit trail, the `service-team-promotion` credential.
- **Abuse cases:**
  - An admin (or a stolen admin token) mints a platform voucher or campaign with no trail: audit first, failure blocks the action.
  - Forcing team-audit down to act unseen: fail closed (`Unavailable`, no row).
  - Forged or spoofed audit entries: the admin principal is never forwarded; only the service principal with `audit.write` writes; `actor_id` is taken from the verified Principal, never the request.
  - Dodging the audit by claiming to be the owner: the rule is the `admin` scope on the verified Principal, never a request field or owner comparison.
  - A seller or buyer trying to reach the admin branches: existing `RequireAdmin` / `RequireSeller` checks run first and are unchanged; refusals write no event.
  - Residual: the insert can fail after a successful audit (for example a duplicate voucher code), leaving one audit entry for an action that did not complete; retries may write another (accepted, no idempotency key in this initiative).
- **Authz rule:** PLATFORM or unspecified `CreateVoucher` requires `admin`; `CreateCampaign` requires seller or admin; `CreateAdCampaign` requires `listing.write`; audit applies whenever `admin` is held (campaigns) or the voucher scope is PLATFORM/unspecified. Audit is written after authn/authz and validation, never for refused calls.
