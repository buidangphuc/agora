# initiative-plan q5-advertised-price
step: 3 (team spec round 1)
worktree: .claude/worktrees/q5-advertised-price (asdlc/q5-advertised-price from asdlc/q4-loop-instrumentation @ 361c6bb5)
decisions (human, 2026-10-16): reject+reprice on price change; flash hold reserve/commit/release in saga; PDP shipping real data, SPX+insurance removed; single-shop parity, stack on Q4.
round 1: validate+spec_lint pass x4; cross_change_check OK; spec-quality: contract fail, promotion fail, order fail (reason-set divergence, hold TTL), frontend pass.
spec-clarify round 1 (2026-10-16, human): hold TTL 15m + commit at Phase B + release returns committed units; FE threshold constants + e2e parity; checkout error page on fee failure; defaults: contract reason set + order, fresh order ids per attempt.
step: 3 (round 2 in progress, all 4 slices)
round 2: spec-quality pass x4; round 3: small fixes (contract wording, promotion purge scope, order Phase B commit order).
step: 5 (LOCK.md pending human)
