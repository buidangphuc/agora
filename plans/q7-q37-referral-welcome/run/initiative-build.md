# initiative-build q7-q37-referral-welcome
| wave | slices | status |
|---|---|---|
| 0 | q7-referral-welcome-contract | merged into feat/q7-q37 |
| 1 | identity-user-registered (W1-T1), order-has-completed-order (W1-T2) | done on feat/q7-q37 (ae200f73, 339e65e8, 3df08fa8); reviews pass; e2e not run |
| 2 | referral-attribution-reward (W2-T1), frontend-referral-invite (W2-T2) | done on feat/q7-q37 (480eb27c, d4371955, 0f2c20fd); reviews pass; e2e not run |
| 3 | promotion-new-user-vouchers (W3-T1), frontend-welcome-checklist (W3-T2) | done on feat/q7-q37 after rebase on origin/main (49202f43, 30b5e5eb, 9c5840a3 + gitops fix); reviews pass; e2e not run |
integration branch: feat/q7-q37 (worktree ../agora-q7-q37); e2e written, not run (stack busy)
next: converge (features-check, full e2e suite, spec_sync --strict per slice, cross_change_check, docs-sync, feature-reviewer + privacy-reviewer) once the local stack is free
converge 2026-11-06 (user scope: q7 fixes only, no full suite): q7 e2e on mainx — identity 23/23 (baseline p95 97.3 ms main vs ~95 ms new), order 13/13, referral_invite 24/24 (S17/S18 under referral-reward overlay), welcome 17/17, new_user_vouchers 11/11 (new-user-voucher overlay), referral_reward 6/6 (overlay). Product fix e719b276 (share-link redirect origin), test fixes 5c5af04f, S18 spec change 80f1255c (VOID not listed; LOCK re-approved). Not done: FEATURES planned→automated flip (spec_sync --strict), full regression suite, feature/privacy reviewers, docs-sync.
