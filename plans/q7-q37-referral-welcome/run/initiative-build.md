# initiative-build q7-q37-referral-welcome
| wave | slices | status |
|---|---|---|
| 0 | q7-referral-welcome-contract | merged into feat/q7-q37 |
| 1 | identity-user-registered (W1-T1), order-has-completed-order (W1-T2) | done on feat/q7-q37 (ae200f73, 339e65e8, 3df08fa8); reviews pass; e2e not run |
| 2 | referral-attribution-reward (W2-T1), frontend-referral-invite (W2-T2) | done on feat/q7-q37 (480eb27c, d4371955, 0f2c20fd); reviews pass; e2e not run |
| 3 | promotion-new-user-vouchers (W3-T1), frontend-welcome-checklist (W3-T2) | done on feat/q7-q37 after rebase on origin/main (49202f43, 30b5e5eb, 9c5840a3 + gitops fix); reviews pass; e2e not run |
integration branch: feat/q7-q37 (worktree ../agora-q7-q37); e2e written, not run (stack busy)
next: converge (features-check, full e2e suite, spec_sync --strict per slice, cross_change_check, docs-sync, feature-reviewer + privacy-reviewer) once the local stack is free
