## 1. Code track

### team-promotion
- [ ] 1.1 store: add `active`, `deactivated_at` to Voucher + migration (default active=true for existing rows); repository `Deactivate` and `UpdateEndsAtQuota` (V1, V4, V8, V9)
- [ ] 1.2 store: map unique-code violation to a typed `ErrDuplicateCode`; add to `errTable` in `internal/rpc/common.go` as `ALREADY_EXISTS` naming `code` (V18)
- [ ] 1.3 `internal/voucher`: `DeactivateVoucher` and `UpdateVoucher`: validate first (NOT_FOUND, state, quota rules, duplicate-code pre-check in `CreateVoucher`), then audit-before-write via `audit.Do` like `CreateVoucher`; actions `voucher.deactivate`, `voucher.update`; quota check + update as one conditional UPDATE (V4-V5a, V8-V16g, V18a, V21b, V22)
- [ ] 1.4 `internal/voucher.validate`: reject inactive with reason `inactive`; keep Commit/Release unchanged (V1-V3)
- [ ] 1.5 `internal/rpc/voucher.go`: handlers `DeactivateVoucher`, `UpdateVoucher` with `requireAdmin`; `Voucher.active`/`deactivated_at` in `voucherToProto` (V6, V16, V17, V27a)
- [ ] 1.6 audit failure maps to `UNAVAILABLE` + `ErrorInfo{promotion.platform, AUDIT_UNAVAILABLE}` for create, deactivate, update (V21)
- [ ] 1.6a repository-level test: after any `UpdateEndsAtQuota`, stored discount type/value, min spend, max discount and scope are identical; concurrent-reservation race test (V16, V16d-V16g)
- [ ] 1.7 tests next to code: `team-promotion internal/voucher` and `internal/rpc` table tests per scenario V1-V18b, V21-V22, V6, V6a, V17, V27a; validation-before-audit ordering tests (V5a, V10a, V11, V13-V15a, V16a, V16b, V18a, V21b)

### team-gateway
- [ ] 1.8 `internal/edge/promotion.go`: forwarders `DeactivateVoucher`, `UpdateVoucher` (write path); method-scope table: admin (V6, V7, V17)
- [ ] 1.9 `internal/edge/forward.go` `toConnectErr`: keep `ErrorInfo` reason on sanitized `Unavailable`; add `PROMOTION_UNAVAILABLE` when the promotion upstream is unreachable (V19, V23-V25)
- [ ] 1.10 gateway unit tests for V7, V19, V23-V25

### team-frontend-admin
- [ ] 1.11 `src/lib/gateway/vouchers.ts`: `deactivateVoucher`, `updateVoucher`, error status split into `duplicate`, `audit_unavailable`, `promotion_unavailable` (V19, V23, V24)
- [ ] 1.12 `src/features/vouchers`: actions, Stop dialog, edit form (end date, quota; quota 0 refused client-side), `voucherMessages.ts` distinct messages, remove immutability text from the create dialog (V20, V26-V33)
- [ ] 1.13 unit tests next to the components (V20, V26-V33)

### team-frontend
- [ ] 1.13a map reason `inactive` to "Mã giảm giá đã ngừng áp dụng." in `src/features/order/VoucherModal.tsx` and `src/features/voucher/VoucherModal.tsx`, with tests next to each (V28a, V28b)

### docs
- [ ] 1.14 remove rows G9, G10, G11 from `fe-platform/docs/gaps.md` (V34, V35)

## 2. E2E track (platform-e2e, owner team-promotion FEATURES.yaml by e2e-author)
- [ ] 2.1 gateway-level: admin stop then buyer-side `ValidateAndReserve` preview returns `inactive`; held order still commits (V1-V3, V7)
- [ ] 2.2 gateway-level: update quota/end date accepted and refused cases (V8-V15, V17)
- [ ] 2.3 gateway-level: duplicate code returns `already_exists` (V18, V19)
- [ ] 2.4 gateway-level: stop team-audit then create/stop/edit shows `AUDIT_UNAVAILABLE`; stop team-promotion shows `PROMOTION_UNAVAILABLE`; nothing changed (V21, V23, V24)
- [ ] 2.5 admin UI: stop, edit, duplicate message, audit-down vs promotion-down messages, non-admin forbidden (V20, V26-V33)
- [ ] 2.6 docs check: gaps.md has no G9-G11 (V34, V35)
