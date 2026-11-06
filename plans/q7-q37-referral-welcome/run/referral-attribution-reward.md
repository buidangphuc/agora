# feature-run referral-attribution-reward
initiative: q7-q37-referral-welcome | base: d82539e7 | worktree: ../agora-q7-q37 (shared with W2-T2, disjoint write-sets) | branch: feat/q7-q37
## E2E written 480eb27c (6 scenarios + compose overlay referral-reward.override.yaml + 46 not-testable), not run (stack busy)
## Tasks 1.1-1.12 batched: approved, rounds=2 (round 1 test-defect TestOrderCompletedAtLifetimeCap setup + 2 lint nits; argocd inline env mirror added, packet widened)
## Gates go vet/test=pass incl. PG+Redpanda testcontainers, lint=pass, gitops unittest 104 OK, red_first=PASS (wave-2 range), features-check=pass, e2e=deferred, decide=ENGINE_DOWN (LLM fallback)
## Review task-reviewer=approve contract-boundary=no BLOCKING auth-scope=PASS
## Open questions for human: DLQ names referral.{identity,order}.dlq vs <source>.<consumer>.dlq convention; registered_at upsert on a later UserRegistered resets account age (LEAST?); Redeem does not lock the referee row (tiny race, no money impact)
## Commit 0f2c20fd on feat/q7-q37 (local, not pushed)
