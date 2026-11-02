## Why

Admins cannot stop or adjust a platform voucher (gap G9), a taken voucher code surfaces as a generic
`INTERNAL` error (G10), and audit-down and promotion-down reach the browser as the same sanitized
`UNAVAILABLE` (G11). Today the create dialog even tells admins a voucher cannot be changed.

initiative: p10-backend-gaps

## What changes

- team-promotion: implement `VoucherService.DeactivateVoucher` and `VoucherService.UpdateVoucher` (contract from
  `p10-gaps-contract`). Soft stop (`active=false`, `deactivated_at`); `ValidateAndReserve` refuses an inactive
  voucher with reason `inactive`; `CommitReservation` / `ReleaseReservation` of existing holds are unchanged.
  `UpdateVoucher` changes only `ends_at` and a raised `quota`.
- team-promotion: map the unique-code violation on `CreateVoucher` to `ALREADY_EXISTS`.
- team-promotion: every admin voucher write (create platform voucher, deactivate, update) writes its audit entry
  first (`voucher.platform_create`, `voucher.deactivate`, `voucher.update`) and fails closed `UNAVAILABLE` with
  `google.rpc.ErrorInfo{domain:"promotion.platform", reason:"AUDIT_UNAVAILABLE"}` when the audit write fails.
- team-gateway: expose `DeactivateVoucher` and `UpdateVoucher` (admin scope) in `internal/edge/promotion.go` and the
  method-scope table; `toConnectErr` keeps the `ErrorInfo` reason (message stays sanitized) and itself produces
  `PROMOTION_UNAVAILABLE` when promotion cannot be reached.
- team-frontend-admin `src/features/vouchers`: stop action, edit end date / quota, distinct messages for duplicate
  code, audit down and promotion down; the create dialog no longer says a voucher cannot be changed.
- team-frontend: both `VoucherModal` components map reason `inactive` to "Mã giảm giá đã ngừng áp dụng." instead of the raw reason.
- Ordering: validation (scope, NOT_FOUND, state, quota rules, duplicate-code pre-check) runs before the audit append; refused calls write no audit entry. Quota rules: admin cannot set quota=0; quota=0 voucher cannot become finite; new quota >= used and >= current; check and update are one conditional UPDATE.
- fe-platform/docs/gaps.md: remove rows G9, G10, G11.

## Non-goals

- Editing discount, min spend, max discount or scope; hard delete; shop-voucher (seller-owned) flows.
- Campaign RPCs (slice `promotion-admin-campaigns`); ErrorInfo for LISTING_UNAVAILABLE (that slice).

## NFR

- Deactivate and update p95 < 300 ms locally (one audit append + one row update); audit-before-write adds one
  synchronous call, no retry loop.
- Availability: audit down or promotion down never leaves a half write; nothing changes on failure.
- Limits: `quota` int64, `ends_at` within the existing create bounds; deactivate is idempotent and O(1).
- A stopped voucher refuses new reservations from the first `ValidateAndReserve` after the write commits.

## Data classification

- Voucher fields (code, quota, window): business data, not personal. Purpose: promotion configuration. No consent
  basis needed; retention: voucher lifetime.
- Audit entry: admin actor id (personal, pseudonymous), action, voucher code, changed fields. Purpose: accountability
  of platform writes; basis: legitimate interest / contract; retention: team-audit policy (unchanged).
- Logs: no actor id or voucher holder data; only `principal_fp` once `gateway-principal-log-fingerprint` lands.

## Threat model

- Assets: voucher state (money-bearing discount budget), audit trail integrity.
- Abuse cases: non-admin stops or raises quota of a platform voucher; admin write without an audit trail; quota
  lowered below usage to break held redemptions; stopping a voucher to strand in-flight orders; error message
  leaking backend details; code enumeration via ALREADY_EXISTS (admin-only RPC, accepted).
- Authz rule: `DeactivateVoucher`, `UpdateVoucher` and platform `CreateVoucher` require scope `admin` at the
  gateway and again in team-promotion (`requireAdmin`); a seller token gets `PERMISSION_DENIED`. Audit is written
  before the change; if the audit write fails the change is not made.
