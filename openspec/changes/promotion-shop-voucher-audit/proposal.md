## Why

`PromotionService.CreateVoucher` with `VOUCHER_SCOPE_SHOP` calls `RequireSeller`, which also admits admins (`internal/interceptor/auth.go` `RequireSeller`), and binds the voucher's seller to the caller's principal id (`internal/handler/voucher.go`). An admin can therefore mint a shop voucher with no audit trail: `service/voucher.go` audits only non-SHOP scope (`voucher.platform_create`). PLATFORM vouchers and flash sales (`flash_sale.admin_create`) already audit admins. Auditors need every admin-created voucher traceable (Q6, Locked 2026-10-18).

initiative: privacy-audit-hardening

## What Changes

- **team-promotion `internal/handler/voucher.go`**: in the SHOP branch, pass whether the verified principal holds `admin` (`interceptor.IsAdmin`) to the service as a new `CreateVoucherParams` field (`AuditAdmin bool`). `AuditActorID` is already the principal id.
- **team-promotion `internal/service/voucher.go`**: when `Scope == SHOP` and `AuditAdmin`, write one team-audit event before the insert via the existing `writeAudit`: `action=voucher.admin_create`, `target_type=voucher`, `target_id=<voucher code>`, `actor_id=<admin id>`, metadata `discount_type`, `value`, `seller_id` (the admin's id, as stored). Nil writer, error or timeout returns `Unavailable` and inserts nothing, emits no `VoucherChanged`.
- Sellers creating their own SHOP vouchers: unchanged, no audit, no dependency on team-audit. PLATFORM and UNSPECIFIED scope keep `voucher.platform_create`.
- Depends on `pah-contract` for the catalogue entry `voucher.admin_create` (vendored `platform/audit/v1` comment, 17 -> 18 actions). No proto, wire or RPC change; no new config (`UPSTREAM_AUDIT_ADDR` and the audit client already exist from `promotion-admin-audit`).
- platform-e2e: scenarios in a new feature `features/promo/promotion_shop_voucher_audit.feature`.

## NFR

- Admin SHOP create adds exactly one team-audit call, bounded by `UPSTREAM_CALL_TIMEOUT_SECONDS` (2 s default), no retry; seller path adds no call and p99 unchanged.
- Availability: admin shop-voucher creation is unavailable while team-audit is down (fail closed, same as other admin paths); seller creation is unaffected.

## Data classification

| field | class | purpose | basis | retention |
|---|---|---|---|---|
| `actor_id` (admin user id) | pseudonymous personal | accountability of admin actions | legitimate interest (security/audit); DPO sign-off is a human item | per team-audit policy (DPO item) |
| `target_id` (voucher code) | non-personal | identify the voucher | same | same |
| metadata `seller_id` (admin id), `discount_type`, `value` | pseudonymous / non-personal | reconstruct the action | same | same |

No new personal data class is introduced; no email, name or IP is written.

## Threat model

- Assets: platform-funded discount money; integrity of the audit trail.
- Abuse cases: an admin (or compromised admin token) mints shop vouchers unnoticed; a caller suppresses the audit by making team-audit unreachable; a seller is mistaken for an admin or vice versa.
- Authz rule: audit is keyed on the verified Principal's `admin` scope from the interceptor, never on request fields; the audit client calls team-audit as service principal `service-team-promotion` and never forwards the admin's principal. Fail closed so no un-audited admin voucher exists. Sellers (no `admin` scope) stay unaudited by design (Q6).
