# LOCK — q7-q37-referral-welcome
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-06
spec: plans/q7-q37-referral-welcome/00-spec.md (Locked 2026-11-06, Clarifications C1-C6 2026-11-06)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q7-referral-welcome-contract | platform-core | [] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 0f7fb4b2d6a3 | [x] |
| identity-user-registered | team-identity | [contract] | 2 | pass | pass (2 PERM warnings accepted) | pass | ENGINE_DOWN (LLM fallback) | 788776f040ec | [x] |
| order-has-completed-order | team-order | [contract] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 1af3920fbd87 | [x] |
| referral-attribution-reward | team-referral | [contract, identity-user-registered] | 2 | pass | pass (2 PERM warnings accepted) | pass | ENGINE_DOWN (LLM fallback) | e094861e64cc | [x] |
| frontend-referral-invite | team-frontend | [referral-attribution-reward] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | ac78fa0df713 | [x] |
| promotion-new-user-vouchers | team-promotion | [contract, identity-user-registered, referral-attribution-reward, order-has-completed-order] | 2 | pass | pass | pass | ENGINE_DOWN (LLM fallback) | 97d28091e268 | [x] |
| frontend-welcome-checklist | team-frontend | [promotion-new-user-vouchers, frontend-referral-invite] | 2 | pass | pass | pass (round 1 fail fixed) | ENGINE_DOWN (LLM fallback) | bc295205b180 | [x] |

## Build order (cross_change_check)
1. q7-referral-welcome-contract (sequential, first)
2. identity-user-registered
3. order-has-completed-order
4. referral-attribution-reward
5. frontend-referral-invite
6. promotion-new-user-vouchers (rebase on PR #97 first)
7. frontend-welcome-checklist

Hot-file pairs that must not run in parallel:
- team-gateway `internal/edge/server.go` / `policy.go`: identity-user-registered ↔ promotion-new-user-vouchers
- `docker-compose.services.yaml` (incl. `redpanda-init`) and `platform-gitops/envs/services/*`: referral-attribution-reward ↔ promotion-new-user-vouchers
- `team-frontend/src/app/(shop)/account/referral/page.tsx`, `src/app/(shop)/vouchers/page.tsx`: frontend-referral-invite ↔ frontend-welcome-checklist

## Cross-change check
pass (`cross_change_check: OK`, after renaming to AuthService.Register, AddressService.ListAddresses, VoucherService.ListMyVouchers)

## Open issues / NEEDS_HUMAN
- decide.py spec gate unusable: kev returns ENGINE_DOWN / usage error ("give exactly one of --state or --part"); every verdict is the spec-quality-reviewer's own ISO 29148/EARS checklist.
- Decided (2026-11-06, approved with LOCK): welcome banner shows "Voucher đang được chuẩn bị" only while the account is < 1 hour old (frontend-welcome-checklist W10/W26).
- DPO sign-off (Decree 13/2023): `referral_code` + `registered_at` in `identity.events`; referrer/referee ids in `referral.events`; `referral_accounts` / `referral_buyer_orders` (legitimate interest, fraud limiting); `agora_ref` cookie and `?ref` in URLs (functional, no consent); `owner_user_id` on vouchers (order retention policy); no erasure path for referral rows (no account-deletion event exists).
- Ops: Vault service credential for team-promotion with scope `order.read`; team-order network policy allowFrom team-promotion; topic retention for `referral.events` / `identity.events` confirmed beyond the broker default (1 week).
- Contract note: `ValidateAndReserveResponse.reason` becomes a stable machine code for `VOUCHER_NOT_ELIGIBLE` / `VOUCHER_PER_USER_LIMIT` (C3); checkout saga unchanged.
- Routed risk: proto (contract), pii (identity, referral, promotion), auth (GetMyAccount + ListMyVouchers gateway routes; HasCompletedOrder must stay unrouted).
- Remaining MINOR reviewer notes (non-blocking) are left to the e2e/test authors: identity wording at spec.md:89/104 and task 1.2 min length; contract C5 line in RedeemReferral comment; promotion DLQ case for referral.events; frontend-welcome design.md 1-hour note and W17 wording.
- Re-approved 2026-11-06 (Bùi Đăng Phúc, converge): frontend-referral-invite S18 rewritten — VOID rewards are not listed (referral-service spec wins); digest 6bcecad50d00 → e7167fb2b231.
- 2026-11-06 converge: traceability-only spec edits (VERIFIED BY lines, identity tasks.md scenario references); no requirement or scenario meaning changed. Digests above updated accordingly.
