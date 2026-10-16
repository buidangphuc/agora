# initiative-build q5-advertised-price
integration: .claude/worktrees/q5-advertised-price (asdlc/q5-advertised-price, base asdlc/q4-loop-instrumentation @ 361c6bb5)

| wave | slices | base | status |
|---|---|---|---|
| 0 | q5-flash-stock-contract | 178a0e5e | merged ed6ee6fb |
| 1 | promotion-flash-stock-hold | ed6ee6fb | merged b4642001 |
| 2 | order-advertised-price | b4642001 | merged f96772f9 |
| 3 | frontend-advertised-price | f96772f9 | merged 6c4ec4a6 |

Per-slice: contract 1 round; promotion 1 (+e2e round 2: replay-id defect, names echo spec titles); order 2 (nil listing fail-closed, cancel flash release own timeout); frontend 1 (spec-clarify Round 2: gateway CalculateShippingFee forwarder added as task 0, re-lock).
Per-slice e2e at merge: promotion 14/14, order 6/6, frontend 6/6 (contract: not-testable, unit + buf).

## Converge (2026-10-16)
- features-check, spec_sync --strict ×4, openspec validate --strict ×4, cross_change_check, validate_plan: pass. tasks.md ticked; spec-clarify Round 3 aligned e2e VERIFIED BY names; LOCK re-locked.
- CI: gofmt/vet/go test -race team-promotion (incl. Postgres on promotion_test), team-order, team-gateway; platform-core lint-proto/breaking + promotion/v1 tests; team-frontend vitest 1617/1617, tsc clean, biome 23 = base.
- ci_local (BASE=asdlc/q4-loop-instrumentation): specs, features, repo, proto, tests-asdlc, security PASS; tdd FAIL on the combined range only (contract impl precedes the first e2e commit, as in Q4) → PASS per PR range (q4..contract, contract..HEAD). red_first's temp worktrees use remote BSR plugins for team-order/team-promotion (no buf.gen.local.yaml) → intermittent "red at head"; rerun with local plugins PASS.
- gitleaks (docker): 27 commits, no leaks.
- Regression subset (33 files, q2 stack): 176 passed, 49 failed; with team-audit up + KAFKA_BROKERS=localhost:19092 rerun: 38 still failing, classification against Q4-base images in progress.
- feature-reviewer per slice: pass (22/22, 14/14, 6/6+43, 6/6+17). privacy-reviewer: frontend needs-human (DPO basis for PDP city display; CDN must not cache authenticated /listing/*).
