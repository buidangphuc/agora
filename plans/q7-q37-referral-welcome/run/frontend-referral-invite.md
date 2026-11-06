# feature-run frontend-referral-invite
initiative: q7-q37-referral-welcome | base: d82539e7 | worktree: ../agora-q7-q37 (shared with W2-T1) | branch: feat/q7-q37
## Built in parallel with referral: code depends only on the merged contract; runtime/e2e depend on referral
## E2E written 480eb27c (22 scenarios + 4 not-testable), not run (stack busy)
## Tasks 1.1-1.8 batched: approved, rounds=1 (orchestrator fixed one biome nit in route.test.ts and the FEATURES.yaml VERIFIED BY titles)
## Gates npm run check (biome+tsc+tokens+vitest 2029)=pass, build=pass, red_first=PASS, docker build not run
## Review task-reviewer=approve contract-boundary=no BLOCKING auth-scope=PASS (frontend actions)
## Commit d4371955 on feat/q7-q37 (local, not pushed)
