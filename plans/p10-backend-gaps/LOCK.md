# LOCK — p10-backend-gaps
status: approved
approved-by: Bùi Đăng Phúc
date: 2026-11-02
spec: plans/p10-backend-gaps/00-spec.md (Locked 2026-11-02, Clarifications 2026-11-02)

## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| p10-gaps-contract | platform-core | [] | 3 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 38740bb45ac1 | [x] |
| gateway-principal-log-fingerprint | team-gateway | [] | 2 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | 79fbfcc092fb | [x] |
| promotion-admin-vouchers | team-promotion | [p10-gaps-contract] | 3 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | e60c71995a88 | [x] |
| promotion-admin-campaigns | team-promotion | [p10-gaps-contract] | 2 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | e1305a027f51 | [x] |
| identity-sso-handoff | team-identity | [p10-gaps-contract] | 4 | pass | pass | pass (r3) | ENGINE_DOWN (LLM fallback) | 17b10a1e0d23 | [x] |
| identity-seller-onboarding | team-identity | [p10-gaps-contract] | 3 | pass | pass | pass (r1) | ENGINE_DOWN (LLM fallback) | a505a3dd02c3 | [x] |
| seller-onboarding-ui | team-frontend-seller | [identity-seller-onboarding] | 3 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | a3e498850d32 | [x] |

## Build order (cross_change_check)
1. p10-gaps-contract (sequential, first: proto + regenerate every consumer by glob)
2. gateway-principal-log-fingerprint (independent; touches gateway interceptors.go/http.go)
3. identity-seller-onboarding → seller-onboarding-ui
4. identity-sso-handoff
5. promotion-admin-vouchers → promotion-admin-campaigns

Hot-file pairs that must not run in parallel:
- promotion-admin-vouchers ↔ promotion-admin-campaigns: gateway promotion.go, forward.go toConnectErr, scope table, admin FE error mapping (vouchers owns the shared mapping, build it first)
- identity-sso-handoff ↔ identity-seller-onboarding: team-identity auth.go, gateway public-RPC list and scope table
- gateway-principal-log-fingerprint ↔ promotion slices: edge/interceptors.go, http.go (ErrorInfo pass-through)
- every feature slice: fe-platform/docs/gaps.md (one row each)
Parallel-safe waves: {sso-handoff, promotion-admin-vouchers, gateway-log} after the contract; then {seller-onboarding, promotion-admin-campaigns}; then seller-onboarding-ui.

## Cross-change check
pass (round 1 ownership violation on login-enforcement-ui fixed by moving the UI into seller-onboarding-ui)

## Open issues / NEEDS_HUMAN
- Tooling: decide.py is ENGINE_DOWN (system python3 lacks PyYAML); the spec-quality-reviewer checklist was the verdict.
- Risk routing: auth (SSO hand-off, BecomeSeller, gateway), pii (hand-off rows, peek username, principal_fp, actor ids), proto.
- Human items:
  - Seller terms text and the `SELLER_TERMS_VERSIONS` / `SELLER_TERMS_VERSION` value (build uses a placeholder "2026-11-02").
  - Vietnamese copy for the admin duplicate/outage messages and the seller denied notice (build uses the drafted copy).
  - DPO basis: admin actor id on campaign idempotency records; `principal_fp` legitimate interest; hand-off rows and peek username.
  - identity.events retention for UserRoleGranted assumed 7 days (Kafka default; no topic config found).
- Assumptions locked in (say so before approving if any is wrong):
  - Voucher: a quota update that loses the race keeps its `voucher.update` audit entry as an attempt record; UpdateVoucher refuses ends_at <= now; error precedence UNAUTHENTICATED > PERMISSION_DENIED > NOT_FOUND > FAILED_PRECONDITION > INVALID_ARGUMENT; admin form hides quota for unlimited vouchers.
  - Campaign: seller-path create needs no idempotency key; page size default 20, max 200; cancelling an ended campaign is FAILED_PRECONDITION.
  - SSO: code 43-char base64url; username shown truncated to 64 chars; a commit failure after a successful audit write leaves an attempt record; dwelling > 60 s on the confirmation page falls back to login; a signed-in target-app user is asked to confirm and the POST replaces that app's session.
  - Seller onboarding: suspended/banned are sent to login (sessions revoked); no startup check that the seller app's terms version matches identity's.
  - Gateway: anonymous requests log `principal_kind=anonymous` with no `principal_fp`; ENV=local uses the SDK's default LOG_KEY_FP_SECRET.
- Out of initiative, done during build as plain fixes: voucher discount "-" in 3 e2e tests (diagnose), `wait_streamed` once in BasePage.
- Deferred: seller/admin ArgoCD, admin ingress VPN/IP, mock audit.

## Re-sign required (converge fixes 2026-11-05)
Reopened after the initiative converge; the human re-ticks each row after reading the change and updates the digest column.
| slice | locked digest | current digest | reason |
|---|---|---|---|
| gateway-principal-log-fingerprint | 79fbfcc092fb | 8bb902009c5a | converge r1 edit (22889aac) |
| promotion-admin-vouchers | e60c71995a88 | 1e112c112168 | VERIFIED BY lines; V35 reworded to "G9-G11 removed, no row added, rest identical" |
| identity-sso-handoff | 17b10a1e0d23 | 808dcd28f66e | VERIFIED BY lines, Referrer-Policy wording; x-principal-sid only to BecomeSeller and the hand-off RPCs; HANDOFF_TTL_OVERRIDE wired |
| identity-seller-onboarding | a505a3dd02c3 | d71c6e69bb4a | BecomeSeller audits auth.become_seller fail-closed (UNAVAILABLE / AUDIT_UNAVAILABLE) |
slices.yaml also changed: identity-sso-handoff depends_on gateway-principal-log-fingerprint (waves 0-5).
