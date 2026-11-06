# feature-run promotion-new-user-vouchers
initiative: q7-q37-referral-welcome | base: f4393fc2 (feat/q7-q37 rebased on origin/main 17d307a6, incl. PR #97-#99) | worktree: ../agora-q7-q37 (shared with W3-T2) | branch: feat/q7-q37
## E2E written 49202f43 (11 scenarios + 28 not-testable + new-user-voucher.override.yaml), not run (stack busy)
## Tasks 1.1-1.8, 2.1, 3.1-3.3 batched: approved, rounds=2 (contract-boundary BLOCKING: gitops topic-init lacked new DLQs/referral.events; fixed by orchestrator; also restored IDENTITY_EVENTS_TOPIC on team-identity compose)
## Decision: ValidateAndReserve order = replay, owner, window/min-spend, per-user, new_user_only, global quota (owner's second use -> VOUCHER_PER_USER_LIMIT per scenario)
## Gates go vet/test incl. PG+Redpanda testcontainers=pass, lint=0, gateway edge=pass, gitops 104 OK, red_first=PASS, e2e=deferred, decide=ENGINE_DOWN (LLM fallback)
## Review task-reviewer=approve contract-boundary=BLOCKING fixed in follow-up commit auth-scope=PASS
## MINOR left: UpdateVoucher two sequential repo calls (partial update on DB failure); sweeper batch of 100 in one tx; stored verdict used when has_code=false; topic/group names are constants not env
## NEEDS_HUMAN: Vault-backed service credential for team-promotion (order.read) in deployed envs
## Commits 9c5840a3 + gitops fix on feat/q7-q37 (local, not pushed)
