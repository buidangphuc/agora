# LOCK — q6-trust-enforcement
status: approved
approved-by: Bùi Đăng Phúc (chat "approve LOCK, build đi")
date: 2026-10-16
spec: plans/q6-trust-enforcement/00-spec.md (Locked 2026-10-16; spec-clarify round 1 decisions appended, human "approve hết" 2026-10-16)
## Slices
| slice | owner | depends_on | rounds | validate | spec_lint | spec-quality | decide (spec) | digest | approve |
|---|---|---|---|---|---|---|---|---|---|
| q6-trust-contract | platform-core | [] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 3f1586638c35 | [x] |
| ai-listing-screen | team-ai | [q6-trust-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | c44ca8175d0a | [x] |
| domain-listing-moderation | team-domain | [ai-listing-screen] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | a2055d22551d | [x] |
| identity-user-enforcement | team-identity | [q6-trust-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | b7d8e2064a5e | [x] |
| domain-seller-enforcement-hold | team-domain | [domain-listing-moderation, identity-user-enforcement] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 0dc251ef5933 | [x] |
| engagement-listing-reports | team-engagement | [q6-trust-contract] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 812ad6556cbf | [x] |
| engagement-dispute-escalation | team-engagement | [engagement-listing-reports] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 0d2d5d85ebc8 | [x] |
| order-dispute-refund | team-order | [engagement-dispute-escalation] | 3 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 7a616acc0b70 | [x] |
| frontend-listing-moderation | team-frontend | [domain-listing-moderation, engagement-listing-reports] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 63cf4408fa6d | [x] |
| frontend-user-enforcement | team-frontend | [identity-user-enforcement] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | f4f549374afc | [x] |
| frontend-dispute-cases | team-frontend | [frontend-listing-moderation, order-dispute-refund] | 2 | pass | pass | pass (r2) | ENGINE_DOWN (LLM fallback) | 2e419195b3c0 | [x] |
## Build order (cross_change_check)
q6-trust-contract -> ai-listing-screen -> domain-listing-moderation -> engagement-listing-reports -> engagement-dispute-escalation -> frontend-listing-moderation -> identity-user-enforcement -> domain-seller-enforcement-hold -> frontend-user-enforcement -> order-dispute-refund -> frontend-dispute-cases
## Cross-change check
pass (cross_change_check: OK)
## Open issues / NEEDS_HUMAN
- Build: in parallel with q10-order-lifecycle, EXCEPT engagement-dispute-escalation, order-dispute-refund and frontend-dispute-cases, which build only after q10 merges to main (they consume OrderReturn.can_escalate and the return window).
- Hot files (serialised): team-gateway `internal/edge/policy.go` and `engagement.go`; team-domain `service/listing.go`, `handler/listing.go`; team-engagement handler/service/migrations; team-frontend `src/features/admin/`, `CockpitView.tsx`, `(shop)/admin/cases/`; platform-e2e `tests/e2e/support/`.
- Routing: proto (contract), auth (admin RPCs, scope removal for restricted users, session revoke), payment (dispute refund through the ReturnRefunded saga), pii (reports, enforcement notes, audit metadata).
- Risk to watch at build: the listing screen now runs on every publish/content edit; the mock default must ALLOW clean text so existing e2e seeds still auto-publish.
- NEEDS_HUMAN before release: DPO (enforcement record basis, 2-year report retention + pseudonymisation, Langfuse traces when a real LLM is on, dispute audit metadata); legal/T&S sign-off on the prohibited-term seed (Decree 52/85); Kafka ACL on `identity.events`.
- Follow-ups: suspension-lift sweeper (lift happens only at next login, A15); a banned seller's open paid orders are not cancelled/refunded by anything yet.
- Clarification decisions (M1–M5, A8–A20, B7–B10, C8–C10) are recorded in 00-spec "Clarifications — spec-clarify round 1" and in each change's design.md.
