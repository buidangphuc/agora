# initiative-build — trust-legal
root: /Users/phuc.buidang/Documents/agora-trust-legal (feat/trust-legal)
integration: .claude/worktrees/trust-legal (asdlc/trust-legal)
wip_limit: 3
| wave | slices | base | status |
|---|---|---|---|
| 0 | trust-legal-contract | asdlc/trust-legal | pending |
| 1 | verification-kyc-documents | asdlc/trust-legal after wave 0 | pending |
| 2 | identity-legal-consent, admin-kyc-queue | asdlc/trust-legal after wave 1 | pending |
| 3 | commerce-user-erasure | asdlc/trust-legal after wave 2 | pending |
| 4 | identity-account-deletion | asdlc/trust-legal after wave 3 | pending |
| 5 | payment-payout-kyc-gate, identity-data-export | asdlc/trust-legal after wave 4 | pending |
| 6 | frontend-legal-pages | asdlc/trust-legal after wave 5 | pending |
| 7 | content-user-erasure, seller-kyc-ui | asdlc/trust-legal after wave 6 | pending |
| 8 | domain-seller-erasure, frontend-account-privacy | asdlc/trust-legal after wave 7 | pending |
| 9 | analytics-user-erasure | asdlc/trust-legal after wave 8 | pending |
Per-slice state: plans/trust-legal/run/<slice>.md

## Wave rationale
- Digests of all 14 slices equal LOCK.md (checked 2026-11-13).
- Contract alone in wave 0. Every later wave respects depends_on (DAG levels: L1 identity-legal-consent, commerce-user-erasure, content-user-erasure, domain-seller-erasure, analytics-user-erasure, verification-kyc-documents; L2 identity-account-deletion, frontend-legal-pages, payment-payout-kyc-gate, admin-kyc-queue; L3 identity-data-export, seller-kyc-ui; L4 frontend-account-privacy).
- Hot files from 00-spec force serialisation: `docker-compose.services.yaml` is written by nine slices (verification-kyc-documents, identity-legal-consent, commerce-user-erasure, identity-account-deletion, payment-payout-kyc-gate, frontend-legal-pages, content-user-erasure, domain-seller-erasure, analytics-user-erasure), so each sits in its own wave (waves 1-9); the four slices without a compose edit ride along.
- `team-gateway/internal/edge/auth.go`: identity-legal-consent (2) -> identity-account-deletion (4) -> identity-data-export (5). `policy.go`: verification-kyc-documents (1) -> identity-account-deletion (4).
- `platform-core/tools/seed-marketplace.sh` + payout steps: identity-legal-consent (2) -> payment-payout-kyc-gate (5) -> seller-kyc-ui (7).
- `team-frontend/src/components/shell/ConsumerShell.tsx`: frontend-legal-pages (6) -> seller-kyc-ui (7) -> frontend-account-privacy (8); `AccountShell.tsx`: seller-kyc-ui (7) -> frontend-account-privacy (8).
- team-payment: commerce-user-erasure (3) -> payment-payout-kyc-gate (5).
- `validate_plan.py plans/trust-legal`: 14 packets, 10 waves, write-sets disjoint per wave.

## Carry-forward
- Worktrees have no platform-e2e/.venv: symlink /Users/phuc.buidang/Documents/agora/platform-e2e/.venv into platform-e2e/ (never commit it). Generated stubs are gitignored: team-gateway `buf generate --template buf.gen.local.yaml`; FE apps `npm ci && npm run proto`.
- Go DB tests: DOCKER_HOST=unix:///Users/phuc.buidang/.orbstack/run/docker.sock GO_SDK_REQUIRE_DOCKER=1.
- LOCK.md open issues A1-A23 and DPO items B are unanswered; workers stop for spec-clarify (human) where an answer changes a scenario. A3: payment-payout-kyc-gate vendors `verification.proto` itself (its write-set holds `team-payment/**`).
