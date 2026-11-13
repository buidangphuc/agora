# initiative-plan run — trust-legal
step reached: 5 (LOCK.md written, status pending; waiting for human sign-off)
spec: plans/trust-legal/00-spec.md (Locked 2026-11-13)
slices: plans/trust-legal/slices.yaml (14 slices)

## Per slice
| slice | owner | author | round | validate | spec_lint | spec-quality | decide | digest |
|---|---|---|---|---|---|---|---|---|
| trust-legal-contract | platform-core | done | 1 | pass | pass | pass | ENGINE_DOWN (decide.py not run to a result) | 425fcffef071 |
| identity-legal-consent | team-identity | done | 1 | pass | pass | pass | ENGINE_DOWN (no yaml, no engine) | 49f0b6b249ba |
| commerce-user-erasure | team-order | done | 1 | pass | pass | pass | ENGINE_DOWN (needs --units and engine) | 260ce34f68b8 |
| identity-account-deletion | team-identity | done | 1 | pass | pass | pass | ENGINE_DOWN | 7b5734cf00b7 |
| identity-data-export | team-identity | done | 1 | pass | pass (2 PERM warnings) | pass | ENGINE_DOWN (no units, no kev engine) | 8017c6e363c0 |
| content-user-erasure | team-engagement | done | 1 | pass | pass | pass | ENGINE_DOWN (no yaml) | 2564d6a1d255 |
| domain-seller-erasure | team-domain | done | 1 | pass | pass (2 PERM warnings) | pass | ENGINE_DOWN (no yaml) | 8202068557de |
| analytics-user-erasure | team-analytics | done | 1 | pass | pass (4 PERM warnings) | pass | ENGINE_DOWN (needs --units/--out, no yaml) | c61a1f27101c |
| verification-kyc-documents | team-verification | done | 1 | pass | pass | pass | ENGINE_DOWN (no yaml) | 3d3e8d5300ca |
| payment-payout-kyc-gate | team-payment | done | 1 | pass | pass (3 PERM warnings) | pass | ENGINE_DOWN (no units, no kev engine) | 77906935b59e |
| frontend-legal-pages | team-frontend | done | 1 | pass | pass | pass | ENGINE_DOWN (no yaml, no units) | 0283b66535d0 |
| frontend-account-privacy | team-frontend | done | 1 | pass | pass | pass | ENGINE_DOWN | d996a770553e |
| seller-kyc-ui | team-frontend-seller | done | 1 | pass | pass | pass | ENGINE_DOWN (no units/out, no engine) | fa44b614550e |
| admin-kyc-queue | team-frontend-admin | done | 1 | pass | pass (1 PERM warning) | pass | ENGINE_DOWN (no units, no kev engine, no yaml) | 46397f7af48a |

decide fallback: spec-quality-reviewer's own checklist is the verdict for every slice.

## Cross-change check
`platform-e2e/.venv/bin/python scripts/asdlc/cross_change_check.py --slices plans/trust-legal/slices.yaml` (2026-11-13): `cross_change_check: OK`, exit 0, no violations.
build order: trust-legal-contract -> analytics-user-erasure -> commerce-user-erasure -> content-user-erasure -> domain-seller-erasure -> identity-legal-consent -> frontend-legal-pages -> identity-account-deletion -> identity-data-export -> frontend-account-privacy -> verification-kyc-documents -> admin-kyc-queue -> payment-payout-kyc-gate -> seller-kyc-ui

## Escalations
none (no slice needed a second round)

## Next
Human answers LOCK.md "Open issues" section A (spec-changing) — via spec-clarify per affected slice, which changes digests and requires recomputing them — then sets status: approved, approved-by, date and ticks each slice. Then initiative-build.
